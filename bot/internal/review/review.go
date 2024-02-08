/*
Copyright 2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package review

import (
	"encoding/json"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"

	"github.com/gravitational/trace"
)

const (
	// Dependabot is the GitHub's bot author/account name.
	// See https://github.com/dependabot.
	Dependabot = "dependabot[bot]"
	// DependabotBatcher is the name of the batcher that groups Dependabot PRs.
	// See https://github.com/Legal-and-General/dependabot-batcher.
	DependabotBatcher = "dependabot-batcher[bot]"
	// RenovateBotPrivate is the name of the app that runs the Renovate action in
	// private repos(teleport.e).
	RenovateBotPrivate = "private-renovate-gha[bot]"
	// RenovateBotPublic is the name of the app that runs the Renovate action in
	// public repos(teleport).
	RenovateBotPublic = "public-renovate-gha[bot]"
	// PostReleaseBot is the name of the bot user that creates post-release PRs
	// such as AMI and docs version updates.
	PostReleaseBot = "teleport-post-release-automation[bot]"
)

var (
	// singleApproverPaths defines paths in cloud or core repos that only require a single approver.
	// The map key is the repo (cloud|teleport|teleport.e) and the value is a list of paths.
	singleApproverPaths = map[string][]string{
		"cloud": []string{
			"deploy/fluxcd/config/values.yaml",
			"deploy/fluxcd/config/*/values.yaml",
			"deploy/fluxcd/config/*/*/values.yaml",
			"deploy/fluxcd/config/*/*/*/values.yaml",
			"deploy/fluxcd/src/platform/*/values.helm.yaml",
			"deploy/fluxcd/src/platform/*/helmrelease.yaml",
			"deploy/fluxcd/src/platform/*/*/values.helm.yaml",
			"deploy/fluxcd/src/platform/*/*/helmrelease.yaml",
		},
	}

	// singleApproverAuthors defines pull request authors in cloud or core repos that only
	// require a single approver. The map key is the repo (cloud|teleport|teleport.e) and the
	// value is a list of authors. BOTS ONLY - DO NOT INCLUDE EMPLOYEE GITHUB HANDLES.
	singleApproverAuthors = map[string][]string{
		"cloud": []string{Dependabot, DependabotBatcher, RenovateBotPrivate, RenovateBotPublic},
	}
)

// SingleApproverPaths returns repository paths that only require a single approver.
func SingleApproverPaths(repository string) []string {
	return singleApproverPaths[repository]
}

// SingleApproverAuthors returns a list of authors that only require a single approver.
func SingleApproverAuthors(repository string) []string {
	return singleApproverAuthors[repository]
}

func isAllowedRobot(author string) bool {
	switch author {
	case Dependabot, DependabotBatcher, RenovateBotPrivate, RenovateBotPublic, PostReleaseBot:
		return true
	}
	return false
}

// Reviewer is a code reviewer.
type Reviewer struct {
	// Owner is true if the reviewer is a code or docs owner (required for all reviews).
	Owner bool `json:"owner"`
	// PreferredOnly is true if the reviewer should only be included in preferred reviewer paths.
	PreferredOnly bool `json:"preferredOnly,omitempty"`
	// PreferredReviewerFor contains a list of file paths that this reviewer
	// should be selected to review.
	PreferredReviewerFor []string `json:"preferredReviewerFor,omitempty"`
}

// Rand allows to override randon number generator in tests.
type Rand interface {
	Intn(int) int
}

// Config holds code reviewer configuration.
type Config struct {
	// Rand is a random number generator. It is not safe for cryptographic
	// operations.
	Rand Rand

	// CodeReviewersOmit is a map of code reviews and code reviewers to omit.
	CodeReviewersOmit map[string]bool `json:"codeReviewersOmit"`

	// CoreReviewers and CloudReviewers defines reviewers for the respositories
	// owned by the core and cloud teams.
	CoreReviewers  map[string]Reviewer `json:"coreReviewers"`
	CloudReviewers map[string]Reviewer `json:"cloudReviewers"`

	// DocsReviewers and DocsReviewersOmit is a map of docs reviews and docs
	// reviewers to omit.
	DocsReviewers     map[string]Reviewer `json:"docsReviewers"`
	DocsReviewersOmit map[string]bool     `json:"docsReviewersOmit"`

	// ReleaseReviewers is a list of reviewers for release PRs.
	ReleaseReviewers []string `json:"releaseReviewers"`

	// Admins are assigned reviews when no others match.
	Admins []string `json:"admins"`
}

// CheckAndSetDefaults checks and sets defaults.
func (c *Config) CheckAndSetDefaults() error {
	if c.Rand == nil {
		c.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	if c.CoreReviewers == nil {
		return trace.BadParameter("missing parameter CoreReviewers")
	}

	if c.CloudReviewers == nil {
		return trace.BadParameter("missing parameter CloudReviewers")
	}

	if c.CodeReviewersOmit == nil {
		return trace.BadParameter("missing parameter CodeReviewersOmit")
	}

	if c.DocsReviewers == nil {
		return trace.BadParameter("missing parameter DocsReviewers")
	}
	if c.DocsReviewersOmit == nil {
		return trace.BadParameter("missing parameter DocsReviewersOmit")
	}

	if c.Admins == nil {
		return trace.BadParameter("missing parameter Admins")
	}

	return nil
}

// Assignments can be used to assign and check code reviewers.
type Assignments struct {
	c *Config
}

// FromString parses JSON formatted configuration and returns assignments.
func FromString(e *env.Environment, reviewers string) (*Assignments, error) {
	var c Config
	if err := json.Unmarshal([]byte(reviewers), &c); err != nil {
		return nil, trace.Wrap(err)
	}

	r, err := New(&c)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return r, nil
}

// New returns new code review assignments.
func New(c *Config) (*Assignments, error) {
	if err := c.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	return &Assignments{
		c: c,
	}, nil
}

// IsInternal checks whether the author of a PR is explicitly
// listed as an internal code or docs reviewer.
func (r *Assignments) IsInternal(author string) bool {
	if isAllowedRobot(author) {
		return true
	}

	_, core := r.c.CoreReviewers[author]
	_, cloud := r.c.CloudReviewers[author]
	_, docs := r.c.DocsReviewers[author]
	return core || cloud || docs
}

// Get will return a list of code reviewers for a given author.
func (r *Assignments) Get(e *env.Environment, changes env.Changes, files []github.PullRequestFile) []string {
	var reviewers []string

	// TODO: consider existing review assignments here
	// https://github.com/gravitational/teleport/issues/10420

	switch {
	case changes.Docs && changes.Code:
		log.Printf("Assign: Found docs and code changes.")
		reviewers = append(reviewers, r.getDocsReviewers(e, files)...)
		reviewers = append(reviewers, r.getCodeReviewers(e, files)...)
	case !changes.Docs && changes.Code:
		log.Printf("Assign: Found code changes.")
		reviewers = append(reviewers, r.getCodeReviewers(e, files)...)
	case changes.Docs && !changes.Code:
		log.Printf("Assign: Found docs changes.")
		reviewers = append(reviewers, r.getDocsReviewers(e, files)...)
	// Strange state, an empty commit? Return admin reviewers.
	case !changes.Docs && !changes.Code:
		log.Printf("Assign: Found no docs or code changes.")
		reviewers = append(reviewers, r.getAdminReviewers(e.Author)...)
	}

	return reviewers
}

func (r *Assignments) repoReviewers(repo string) map[string]Reviewer {
	switch repo {
	case env.TeleportRepo, env.TeleportERepo:
		return r.c.CoreReviewers
	case env.CloudRepo:
		return r.c.CloudReviewers
	}
	return map[string]Reviewer{}
}

func (r *Assignments) getReleaseReviewers() []string {
	return r.c.ReleaseReviewers
}

func (r *Assignments) getDocsReviewers(e *env.Environment, files []github.PullRequestFile) []string {
	// See if any code reviewers are designated preferred reviewers for one of
	// the changed docs files. If so, add them as docs reviewers.
	repoReviewers := r.repoReviewers(e.Repository)
	a, b := getReviewerSets(e.Author, repoReviewers, r.c.CodeReviewersOmit)
	prefCodeReviewers := r.getAllPreferredReviewers(repoReviewers, append(a, b...), files)

	// Get the docs reviewer pool, which does not depend on the files
	// changed by a pull request.
	docsA, docsB := getReviewerSets(e.Author, r.c.DocsReviewers, r.c.DocsReviewersOmit)
	reviewers := append(prefCodeReviewers, append(docsA, docsB...)...)

	// If no docs reviewers were assigned, assign admin reviews.
	if len(reviewers) == 0 {
		log.Println("No docs reviewers found. Assigning admin reviewers.")
		return r.getAdminReviewers(e.Author)
	}
	return reviewers
}

func (r *Assignments) getCodeReviewers(e *env.Environment, files []github.PullRequestFile) []string {
	reviewers := r.repoReviewers(e.Repository)

	// Obtain full sets of reviewers.
	setA, setB := r.getCodeReviewerSets(e)

	// Sort the sets to get predictable order. It doesn't matter in real use
	// because selection is randomized but helps in tests.
	sort.Strings(setA)
	sort.Strings(setB)

	// See if there are preferred reviewers for the changeset.
	preferredSetA := r.getPreferredReviewers(reviewers, setA, files)
	preferredSetB := r.getPreferredReviewers(reviewers, setB, files)

	// All preferred reviewers should be requested reviews. If there are none,
	// pick from the overall set at random.
	resultingSetA := preferredSetA
	if len(resultingSetA) == 0 {
		// Only include reviewers from setA whose preferredOnly field is false.
		setA = filterPreferredOnly(reviewers, setA, false)
		resultingSetA = append(resultingSetA, setA[r.c.Rand.Intn(len(setA))])
	}
	resultingSetB := preferredSetB
	if len(resultingSetB) == 0 {
		// Only include reviewers from setB whose preferredOnly field is false.
		setB = filterPreferredOnly(reviewers, setB, false)
		resultingSetB = append(resultingSetB, setB[r.c.Rand.Intn(len(setB))])
	}

	return append(resultingSetA, resultingSetB...)
}

// getPreferredReviewers returns a list of reviewers that would be preferrable
// to review the provided changeset. Returns at most one preferred reviewer per
// file path.
func (r *Assignments) getPreferredReviewers(teamReviewers map[string]Reviewer, set []string, files []github.PullRequestFile) (preferredReviewers []string) {
	// To avoid assigning too many reviewers iterate over paths that we have
	// preferred reviewers for and see if any of them are among the changeset.
	coveredPaths := make(map[string]struct{})
	for path, reviewers := range r.getPreferredReviewersMap(teamReviewers, set) {
		if _, ok := coveredPaths[path]; ok {
			continue
		}
		for _, file := range files {
			if strings.HasPrefix(file.Name, path) {
				reviewer := reviewers[r.c.Rand.Intn(len(reviewers))]
				log.Printf("Picking %v as preferred reviewer for %v which matches %v.", reviewer, file.Name, path)
				preferredReviewers = append(preferredReviewers, reviewer)
				for _, path := range teamReviewers[reviewer].PreferredReviewerFor {
					coveredPaths[path] = struct{}{}
				}
				break
			}
		}
	}
	return preferredReviewers
}

// getAllPreferredReviewers returns a list of reviewers that would be
// preferrable to review the provided changeset. Includes all preferred
// reviewers for each file path in the changeset.
func (r *Assignments) getAllPreferredReviewers(reviewers map[string]Reviewer, set []string, files []github.PullRequestFile) (preferredReviewers []string) {
	// Check each key in the preferred reviewer map, which is a file path
	// that reviewers are assigned to. For any file names in the changeset
	// that begin with that file path, add the reviewers for that pile path
	// to the set of preferred reviewers. Look up each reviewer in a map to
	// avoid duplication.
	assigned := make(map[string]struct{})
	for path, reviewers := range r.getPreferredReviewersMap(reviewers, set) {
		for _, file := range files {
			if !strings.HasPrefix(file.Name, path) {
				continue
			}
			for _, rev := range reviewers {
				if _, ok := assigned[rev]; ok {
					continue
				}
				assigned[rev] = struct{}{}
				preferredReviewers = append(preferredReviewers, rev)
			}
		}
	}
	return preferredReviewers
}

// getPreferredReviewersMap builds a map of preferred reviewers for file paths.
func (r *Assignments) getPreferredReviewersMap(reviewers map[string]Reviewer, set []string) map[string][]string {
	m := make(map[string][]string)
	for _, name := range set {
		if reviewer, ok := reviewers[name]; ok {
			for _, path := range reviewer.PreferredReviewerFor {
				m[path] = append(m[path], name)
			}
		}
	}
	return m
}

// getAdminReviewers returns the list of admin reviewers. Respects code
// reviewer omits and removes admins in omit list from reviews.
func (r *Assignments) getAdminReviewers(author string) []string {
	var reviewers []string
	for _, v := range r.c.Admins {
		if v == author {
			continue
		}
		if _, ok := r.c.CodeReviewersOmit[v]; ok {
			continue
		}
		reviewers = append(reviewers, v)
	}
	return reviewers
}

func (r *Assignments) getCodeReviewerSets(e *env.Environment) ([]string, []string) {
	// Internal non-Core contributors get assigned from the admin reviewer set.
	// Admins will review, triage, and re-assign.
	if !r.IsInternal(e.Author) {
		reviewers := r.getAdminReviewers(e.Author)
		n := len(reviewers) / 2
		return reviewers[:n], reviewers[n:]
	}
	return getReviewerSets(e.Author, r.repoReviewers(e.Repository), r.c.CodeReviewersOmit)
}

// CheckExternal requires two admins have approved.
func (r *Assignments) CheckExternal(author string, reviews []github.Review) error {
	log.Printf("Check: Found external author %q.", author)

	reviewers := r.GetAdminCheckers(author)

	if checkN(reviewers, reviews) > 1 {
		return nil
	}
	return trace.BadParameter("at least two approvals required from %v", reviewers)
}

// CheckInternal will verify if required reviewers have approved. Checks if
// docs and if each set of code reviews have approved. Admin approvals bypass
// all checks.
func (r *Assignments) CheckInternal(e *env.Environment, reviews []github.Review, changes env.Changes, files []github.PullRequestFile) error {
	log.Printf("Check: Found internal author %v.", e.Author)

	// Skip checks if admins have approved.
	if check(r.GetAdminCheckers(e.Author), reviews) {
		log.Println("Check: Detected admin approval, skipping the rest of checks.")
		return nil
	}

	if changes.Code && changes.Large {
		log.Println("Check: Detected large PR, requiring admin approval")
		if !check(r.GetAdminCheckers(e.Author), reviews) {
			return trace.BadParameter("this PR is large and requires admin approval to merge")
		}
	}

	if changes.Release {
		log.Println("Check: Detected release PR.")
		if err := r.checkInternalReleaseReviews(reviews); err != nil {
			return trace.Wrap(err)
		}
		return nil
	}

	switch {
	case changes.Docs && changes.Code:
		log.Printf("Check: Found docs and code changes.")
		if err := r.checkInternalDocsReviews(e, reviews, files); err != nil {
			return trace.Wrap(err)
		}
		if err := r.checkInternalCodeReviews(e, changes, reviews); err != nil {
			return trace.Wrap(err)
		}
	case !changes.Docs && changes.Code:
		log.Printf("Check: Found code changes.")
		if err := r.checkInternalCodeReviews(e, changes, reviews); err != nil {
			return trace.Wrap(err)
		}
	case changes.Docs && !changes.Code:
		log.Printf("Check: Found docs changes.")
		if err := r.checkInternalDocsReviews(e, reviews, files); err != nil {
			return trace.Wrap(err)
		}
	// Strange state, an empty commit? Check admins.
	case !changes.Docs && !changes.Code:
		log.Printf("Check: Found no docs or code changes.")
		if checkN(r.GetAdminCheckers(e.Author), reviews) < 2 {
			return trace.BadParameter("requires two admin approvals")
		}
	}

	return nil
}

func (r *Assignments) checkInternalReleaseReviews(reviews []github.Review) error {
	reviewers := r.getReleaseReviewers()
	if len(reviewers) == 0 {
		return trace.BadParameter("list of release reviewers is empty, check releaseReviewers field in the reviewers map")
	}

	if check(reviewers, reviews) {
		return nil
	}

	return trace.BadParameter("requires at least one approval from %v", reviewers)
}

// checkInternalDocsReviews checks whether docs review requirements are satisfied
// for a PR authored by an internal employee
func (r *Assignments) checkInternalDocsReviews(e *env.Environment, reviews []github.Review, files []github.PullRequestFile) error {
	reviewers := r.getDocsReviewers(e, files)

	if check(reviewers, reviews) {
		return nil
	}

	return trace.BadParameter("requires at least one approval from %v", reviewers)
}

// checkInternalCodeReviews checks whether code review requirements are satisfied
// for a PR authored by an internal employee
func (r *Assignments) checkInternalCodeReviews(e *env.Environment, changes env.Changes, reviews []github.Review) error {
	setA, setB := getReviewerSets(e.Author, r.repoReviewers(e.Repository), r.c.CodeReviewersOmit)

	// PRs can be approved if you either have multiple code owners that approve
	// or code owner and code reviewer. An exception is for PRs that
	// only modify paths that require a single approver.
	a := checkN(setA, reviews)
	b := checkN(setB, reviews)
	switch {
	case changes.ApproverCount == 1 && (a >= 1 || b >= 1):
		return nil
	case a >= changes.ApproverCount:
		return nil
	case a > 0 && b > 0 && a+b >= changes.ApproverCount:
		return nil
	}

	return trace.BadParameter("at least one approval required from each set %v %v", setA, setB)
}

// GetAdminCheckers returns list of admins approvers.
func (r *Assignments) GetAdminCheckers(author string) []string {
	var reviewers []string
	for _, v := range r.c.Admins {
		if v == author {
			continue
		}
		reviewers = append(reviewers, v)
	}
	return reviewers
}

func getReviewerSets(author string, reviewers map[string]Reviewer, reviewersOmit map[string]bool) ([]string, []string) {
	var setA []string
	var setB []string

	for k, v := range reviewers {
		// Skip over reviewers that are marked as omit.
		if _, ok := reviewersOmit[k]; ok {
			continue
		}
		// Skip author, can't assign/review own PR.
		if k == author {
			continue
		}

		if v.Owner {
			setA = append(setA, k)
		} else {
			setB = append(setB, k)
		}
	}

	return setA, setB
}

func check(reviewers []string, reviews []github.Review) bool {
	return checkN(reviewers, reviews) > 0
}

func checkN(reviewers []string, reviews []github.Review) int {
	r := reviewsByAuthor(reviews)

	var n int
	for _, reviewer := range reviewers {
		if state, ok := r[reviewer]; ok && state == Approved {
			n++
		}
	}
	return n
}

func reviewsByAuthor(reviews []github.Review) map[string]string {
	m := map[string]string{}

	for _, review := range reviews {
		// Always pick up the last submitted review from each reviewer.
		if state, ok := m[review.Author]; ok {
			// If the reviewer left comments after approval, skip this review.
			if review.State == Commented && state == Approved {
				continue
			}
		}
		m[review.Author] = review.State
	}

	return m
}

// filterPreferredOnly returns all names in set whose preferredOnly field is set to the preferredOnly parameter.
func filterPreferredOnly(reviewers map[string]Reviewer, set []string, preferredOnly bool) []string {
	filtered := make([]string, 0, len(set))
	for _, name := range set {
		reviewer, ok := reviewers[name]
		if !ok {
			continue
		}
		if reviewer.PreferredOnly == preferredOnly {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

const (
	// Commented is a code review where the reviewer has left comments only.
	Commented = "COMMENTED"
	// Approved is a code review where the reviewer has approved changes.
	Approved = "APPROVED"
	// ChangesRequested is a code review where the reviewer has requested changes.
	ChangesRequested = "CHANGES_REQUESTED"
)
