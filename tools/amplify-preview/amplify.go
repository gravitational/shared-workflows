package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/amplify"
	"github.com/aws/aws-sdk-go-v2/service/amplify/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
)

var (
	errBranchNotFound = errors.New("Branch not found")
	errNoJobForBranch = errors.New("Current branch has no jobs")
)

const (
	logKeyAppID      = "appID"
	logKeyBranchName = "branchName"
	logKeyJobID      = "jobID"

	amplifyMarkdownHeader = "Amplify deployment status"
	amplifyDefaultDomain  = "amplifyapp.com"
)

type AmplifyPreview struct {
	appIDs []string
	client *amplify.Client
}

type aggregatedError struct {
	perAppErr map[string]error
	message   string
}

func (amp *AmplifyPreview) FindExistingBranch(ctx context.Context, branchName string) (*types.Branch, error) {
	type resp struct {
		appID string
		data  *amplify.GetBranchOutput
		err   error
	}
	resultCh := make(chan resp, len(amp.appIDs))

	for _, appID := range amp.appIDs {
		go func() {
			branch, err := amp.client.GetBranch(ctx, &amplify.GetBranchInput{
				AppId:      aws.String(appID),
				BranchName: aws.String(branchName),
			})
			resultCh <- resp{
				appID: appID,
				data:  branch,
				err:   err,
			}

		}()
	}

	failedResp := aggregatedError{
		perAppErr: map[string]error{},
		message:   "failed to fetch branch",
	}

	for resp := range resultCh {
		var errNotFound *types.ResourceNotFoundException
		if errors.As(resp.err, &errNotFound) {
			slog.Debug("Branch not found", logKeyAppID, resp.appID, logKeyBranchName, branchName)
			continue
		} else if resp.err != nil {
			failedResp.perAppErr[resp.appID] = resp.err
			continue
		}

		if resp.data != nil {
			return resp.data.Branch, nil
		}
	}

	if err := failedResp.Error(); err != nil {
		return nil, err
	}

	return nil, errBranchNotFound
}

func (amp *AmplifyPreview) CreateBranch(ctx context.Context, branchName string) (*types.Branch, error) {
	failedResp := aggregatedError{
		perAppErr: map[string]error{},
		message:   "failed to create branch",
	}

	for _, appID := range amp.appIDs {
		resp, err := amp.client.CreateBranch(ctx, &amplify.CreateBranchInput{
			AppId:           aws.String(appID),
			BranchName:      aws.String(branchName),
			Description:     aws.String("Branch generated for PR TODO"),
			Stage:           types.StagePullRequest,
			EnableAutoBuild: aws.Bool(true),
		})

		var errLimitExceeded *types.LimitExceededException
		if errors.As(err, &errLimitExceeded) {
			slog.Debug("Reached branches limit", logKeyAppID, appID)
		} else if err != nil {
			failedResp.perAppErr[appID] = err
		}

		if resp != nil {
			slog.Info("Successfully created branch", logKeyAppID, appID, logKeyBranchName, resp.Branch.BranchName, logKeyJobID)
			return resp.Branch, nil
		}
	}

	return nil, failedResp.Error()
}

func (amp *AmplifyPreview) StartJob(ctx context.Context, branch *types.Branch) (*types.JobSummary, error) {
	appID, err := appIDFromBranchARN(*branch.BranchArn)
	if err != nil {
		return nil, err
	}

	resp, err := amp.client.StartJob(ctx, &amplify.StartJobInput{
		AppId:      &appID,
		BranchName: branch.BranchName,
		JobType:    types.JobTypeRelease,
		JobReason:  aws.String("Initial job for PR-xxx"),
	})

	if err != nil {
		return nil, err
	}

	slog.Info("Successfully started job", logKeyAppID, appID, logKeyBranchName, branch.BranchName, logKeyJobID, resp.JobSummary.JobId)

	return resp.JobSummary, nil

}

func (amp *AmplifyPreview) GetJob(ctx context.Context, branch *types.Branch, jobID *string) (*types.JobSummary, error) {
	appID, err := appIDFromBranchARN(*branch.BranchArn)
	if err != nil {
		return nil, err
	}

	if jobID == nil {
		jobID = branch.ActiveJobId
	}

	if jobID != nil {
		resp, err := amp.client.GetJob(ctx, &amplify.GetJobInput{
			AppId:      aws.String(appID),
			BranchName: branch.BranchName,
			JobId:      jobID,
		})
		if err != nil {
			return nil, err
		}

		return resp.Job.Summary, nil
	}

	return nil, errNoJobForBranch
}

func appIDFromBranchARN(branchArn string) (string, error) {
	parsedArn, err := arn.Parse(branchArn)
	if err != nil {
		return "", err
	}

	if arnParts := strings.Split(parsedArn.Resource, "/"); len(arnParts) > 2 {
		return arnParts[1], nil
	}

	return "", fmt.Errorf("Invalid branch ARN")
}

func (err aggregatedError) Error() error {
	if len(err.perAppErr) == 0 {
		return nil
	}

	var msg strings.Builder
	for k, v := range err.perAppErr {
		msg.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}

	return fmt.Errorf("%s for apps:\n\t%s", err.message, msg.String())
}

func amplifyJobToMarkdown(job *types.JobSummary, branch *types.Branch) string {
	var mdTableHeader = [...]string{"Branch", "Status", "Preview", "Updated (UTC)"}
	var commentBody strings.Builder
	appID, _ := appIDFromBranchARN(*branch.BranchArn)

	commentBody.WriteString(amplifyMarkdownHeader)
	commentBody.WriteByte('\n')

	commentBody.WriteString(strings.Join(mdTableHeader[:], " | "))
	commentBody.WriteByte('\n')
	commentBody.WriteString(strings.TrimSuffix(strings.Repeat("---------|", len(mdTableHeader)), "|"))
	commentBody.WriteByte('\n')
	commentBody.WriteString(*branch.BranchName)
	commentBody.WriteString(" | ")
	commentBody.WriteString(string(job.Status))
	commentBody.WriteString(" | ")
	commentBody.WriteString(strings.Join([]string{*branch.DisplayName, appID, amplifyDefaultDomain}, "."))
	commentBody.WriteString(" | ")

	if job.EndTime == nil {
		commentBody.WriteString(job.StartTime.String())
	} else {
		commentBody.WriteString(job.EndTime.String())
	}
	commentBody.WriteByte('\n')

	return commentBody.String()
}
