package manager

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/ptr"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
)

// regionMock is a per-region EC2API used by manager tests. Each region's
// configuration (regions list to expose via DescribeRegions, images to expose
// via DescribeImages, errors, in-flight tracking, write-call captures) is set
// up before the factory is invoked.
//
// Manager calls DescribeRegions on the bootstrap region's client, so the mock
// for that region also has describeRegionsOut populated. Other regions' mocks
// leave it nil and panic if asked.
type regionMock struct {
	region string

	// For the bootstrap region: regions to surface via DescribeRegions.
	// Nil → DescribeRegions panics.
	describeRegionsOut []string

	describeImagesOut *ec2.DescribeImagesOutput
	describeImagesErr error

	callsMu           sync.Mutex
	enableDeprecation []*ec2.EnableImageDeprecationInput
	modifyAttribute   []*ec2.ModifyImageAttributeInput
	deregisterImage   []*ec2.DeregisterImageInput
	deleteSnapshot    []*ec2.DeleteSnapshotInput

	// inFlight + maxInFlight are pointers to shared atomics used by the
	// concurrency-limit test to observe peak overlap across all regions.
	inFlight    *int64
	maxInFlight *int64

	// describeDelay slows down DescribeImages so the concurrency test can
	// reliably observe overlap.
	describeDelay time.Duration
}

func (m *regionMock) DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	if m.describeRegionsOut == nil {
		panic("DescribeRegions called on a non-bootstrap regionMock: " + m.region)
	}
	out := &ec2.DescribeRegionsOutput{}
	for _, r := range m.describeRegionsOut {
		out.Regions = append(out.Regions, types.Region{
			RegionName:  ptr.String(r),
			OptInStatus: ptr.String("opt-in-not-required"),
		})
	}
	return out, nil
}

func (m *regionMock) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	if m.inFlight != nil {
		cur := atomic.AddInt64(m.inFlight, 1)
		defer atomic.AddInt64(m.inFlight, -1)
		if m.maxInFlight != nil {
			for {
				old := atomic.LoadInt64(m.maxInFlight)
				if cur <= old {
					break
				}
				if atomic.CompareAndSwapInt64(m.maxInFlight, old, cur) {
					break
				}
			}
		}
	}
	if m.describeDelay > 0 {
		time.Sleep(m.describeDelay)
	}
	if m.describeImagesErr != nil {
		return nil, m.describeImagesErr
	}
	if m.describeImagesOut == nil {
		return &ec2.DescribeImagesOutput{}, nil
	}
	return m.describeImagesOut, nil
}

func (m *regionMock) EnableImageDeprecation(ctx context.Context, in *ec2.EnableImageDeprecationInput, _ ...func(*ec2.Options)) (*ec2.EnableImageDeprecationOutput, error) {
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	m.enableDeprecation = append(m.enableDeprecation, in)
	return &ec2.EnableImageDeprecationOutput{}, nil
}

func (m *regionMock) ModifyImageAttribute(ctx context.Context, in *ec2.ModifyImageAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifyImageAttributeOutput, error) {
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	m.modifyAttribute = append(m.modifyAttribute, in)
	return &ec2.ModifyImageAttributeOutput{}, nil
}

func (m *regionMock) DeregisterImage(ctx context.Context, in *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	m.deregisterImage = append(m.deregisterImage, in)
	return &ec2.DeregisterImageOutput{}, nil
}

func (m *regionMock) DeleteSnapshot(ctx context.Context, in *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	m.deleteSnapshot = append(m.deleteSnapshot, in)
	return &ec2.DeleteSnapshotOutput{}, nil
}

// stsMock is a minimal STS fake.
type stsMock struct {
	account string
	err     error
}

func (s *stsMock) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &sts.GetCallerIdentityOutput{Account: ptr.String(s.account)}, nil
}

// fakeFactory looks up the region in perRegion. Returns nil-safe panic for
// unknown regions so tests fail loudly on misconfiguration.
func fakeFactory(perRegion map[string]*regionMock) ec2iface.RegionalClientFactory {
	return func(region string) ec2iface.EC2API {
		c, ok := perRegion[region]
		if !ok {
			panic("no mock for region " + region)
		}
		return c
	}
}

var errBoom = errors.New("boom")
