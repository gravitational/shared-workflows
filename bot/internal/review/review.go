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
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"

	"github.com/gravitational/trace"
)

// Reviewer is a code reviewer.
type Reviewer struct {
	// Team the reviewer belongs to.
	Team string `json:"team"`
	// Owner is true if the reviewer is a code or docs owner (required for all reviews).
	Owner bool `json:"owner"`
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

	// CodeReviewers and CodeReviewersOmit is a map of code reviews and code
	// reviewers to omit.
	CodeReviewers     map[string]Reviewer `json:"codeReviewers"`
	CodeReviewersOmit map[string]bool     `json:"codeReviewersOmit"`

	// DocsReviewers and DocsReviewersOmit is a map of docs reviews and docs
	// reviewers to omit.
	DocsReviewers     map[string]Reviewer `json:"docsReviewers"`
	DocsReviewersOmit map[string]bool     `json:"docsReviewersOmit"`

	// Admins are assigned reviews when no others match.
	Admins []string `json:"admins"`
}

// CheckAndSetDefaults checks and sets defaults.
func (c *Config) CheckAndSetDefaults() error {
	if c.Rand == nil {
		c.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	if c.CodeReviewers == nil {
		return trace.BadParameter("missing parameter CodeReviewers")
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
func FromString(reviewers string) (*Assignments, error) {
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

// IsInternal returns if the author of a PR is internal.
func (r *Assignments) IsInternal(author string) bool {
	_, code := r.c.CodeReviewers[author]
	_, docs := r.c.DocsReviewers[author]
	return code || docs
}

// Get will return a list of code reviewers for a given author.
func (r *Assignments) Get(e *env.Environment, docs bool, code bool, files []github.PullRequestFile) []string {
	var reviewers []string

	// TODO: consider existing review assignments here
	// https://github.com/gravitational/teleport/issues/10420

	switch {
	case docs && code:
		log.Printf("Assign: Found docs and code changes.")
		reviewers = append(reviewers, r.getDocsReviewers(e.Author)...)
		reviewers = append(reviewers, r.getCodeReviewers(e, files)...)
	case !docs && code:
		log.Printf("Assign: Found code changes.")
		reviewers = append(reviewers, r.getCodeReviewers(e, files)...)
	case docs && !code:
		log.Printf("Assign: Found docs changes.")
		reviewers = append(reviewers, r.getDocsReviewers(e.Author)...)
	// Strange state, an empty commit? Return admin reviewers.
	case !docs && !code:
		log.Printf("Assign: Found no docs or code changes.")
		reviewers = append(reviewers, r.getAdminReviewers(e.Author)...)
	}

	return reviewers
}

func (r *Assignments) getDocsReviewers(author string) []string {
	setA, setB := getReviewerSets(author, "Core", r.c.DocsReviewers, r.c.DocsReviewersOmit)
	reviewers := append(setA, setB...)

	// If no docs reviewers were assigned, assign admin reviews.
	if len(reviewers) == 0 {
		return r.getAdminReviewers(author)
	}
	return reviewers
}

func (r *Assignments) getCodeReviewers(e *env.Environment, files []github.PullRequestFile) []string {
	// Obtain full sets of reviewers.
	setA, setB := r.getCodeReviewerSets(e)

	// Sort the sets to get predictable order. It doesn't matter in real use
	// because selection is randomized but helps in tests.
	sort.Strings(setA)
	sort.Strings(setB)

	// See if there are preferred reviewers for the changeset.
	preferredSetA := r.getPreferredReviewers(setA, files)
	preferredSetB := r.getPreferredReviewers(setB, files)

	// All preferred reviewers should be requested reviews. If there are none,
	// pick from the overall set at random.
	resultingSetA := preferredSetA
	if len(resultingSetA) == 0 {
		resultingSetA = append(resultingSetA, setA[r.c.Rand.Intn(len(setA))])
	}
	resultingSetB := preferredSetB
	if len(resultingSetB) == 0 {
		resultingSetB = append(resultingSetB, setB[r.c.Rand.Intn(len(setB))])
	}

	return append(resultingSetA, resultingSetB...)
}

// getPreferredReviewers returns a list of reviewers that would be preferrable
// to review the provided changeset.
func (r *Assignments) getPreferredReviewers(set []string, files []github.PullRequestFile) (preferredReviewers []string) {
	// To avoid assigning too many reviewers iterate over paths that we have
	// preferred reviewers for and see if any of them are among the changeset.
	for path, reviewers := range r.getPreferredReviewersMap(set) {
		for _, file := range files {
			if strings.HasPrefix(file.Name, path) {
				reviewer := reviewers[r.c.Rand.Intn(len(reviewers))]
				log.Printf("Picking %v as preferred reviewer for %v which matches %v.", reviewer, file.Name, path)
				preferredReviewers = append(preferredReviewers, reviewer)
				break
			}
		}
	}
	return preferredReviewers
}

// getPreferredReviewersMap builds a map of preferred reviewers for file paths.
func (r *Assignments) getPreferredReviewersMap(set []string) map[string][]string {
	m := make(map[string][]string)
	for _, name := range set {
		if reviewer, ok := r.c.CodeReviewers[name]; ok {
			for _, path := range reviewer.PreferredReviewerFor {
				m[path] = append(m[path], name)
			}
		}
	}
	return m
}

func (r *Assignments) getAdminReviewers(author string) []string {
	var reviewers []string
	for _, v := range r.c.Admins {
		if v == author {
			continue
		}
		reviewers = append(reviewers, v)
	}
	return reviewers
}

func (r *Assignments) getCodeReviewerSets(e *env.Environment) ([]string, []string) {
	// Internal non-Core contributors get assigned from the admin reviewer set.
	// Admins will review, triage, and re-assign.

	v, ok := r.c.CodeReviewers[e.Author]
	if !ok || v.Team == internalTeam {
		reviewers := r.getAdminReviewers(e.Author)
		n := len(reviewers) / 2
		return reviewers[:n], reviewers[n:]
	}

	team := v.Team

	// Teams do their own internal reviews
	switch e.Repository {
	case teleportRepo:
		team = coreTeam
	case cloudRepo:
		team = cloudTeam
	}

	return getReviewerSets(e.Author, team, r.c.CodeReviewers, r.c.CodeReviewersOmit)
}

// CheckExternal requires two admins have approved.
func (r *Assignments) CheckExternal(author string, reviews []github.Review) error {
	log.Printf("Check: Found external author %v.", author)

	reviewers := r.getAdminReviewers(author)

	if checkN(reviewers, reviews) > 1 {
		return nil
	}
	return trace.BadParameter("at least two approvals required from %v", reviewers)
}

// CheckInternal will verify if required reviewers have approved. Checks if
// docs and if each set of code reviews have approved. Admin approvals bypass
// all checks.
func (r *Assignments) CheckInternal(e *env.Environment, reviews []github.Review, docs bool, code bool, large bool) error {
	log.Printf("Check: Found internal author %v.", e.Author)

	// Skip checks if admins have approved.
	if check(r.getAdminReviewers(e.Author), reviews) {
		return nil
	}

	if code && large {
		log.Println("Check: Detected large PR, requiring admin approval")
		if !check(r.getAdminReviewers(e.Author), reviews) {
			return trace.BadParameter("this PR is large and requires admin approval to merge")
		}
	}

	switch {
	case docs && code:
		log.Printf("Check: Found docs and code changes.")
		if err := r.checkDocsReviews(e.Author, reviews); err != nil {
			return trace.Wrap(err)
		}
		if err := r.checkCodeReviews(e, reviews); err != nil {
			return trace.Wrap(err)
		}
	case !docs && code:
		log.Printf("Check: Found code changes.")
		if err := r.checkCodeReviews(e, reviews); err != nil {
			return trace.Wrap(err)
		}

	case docs && !code:
		log.Printf("Check: Found docs changes.")
		if err := r.checkDocsReviews(e.Author, reviews); err != nil {
			return trace.Wrap(err)
		}
	// Strange state, an empty commit? Check admins.
	case !docs && !code:
		log.Printf("Check: Found no docs or code changes.")
		if checkN(r.getAdminReviewers(e.Author), reviews) < 2 {
			return trace.BadParameter("requires two admin approvals")
		}
	}

	return nil
}

func (r *Assignments) checkDocsReviews(author string, reviews []github.Review) error {
	reviewers := r.getDocsReviewers(author)

	if check(reviewers, reviews) {
		return nil
	}

	return trace.BadParameter("requires at least one approval from %v", reviewers)
}

func (r *Assignments) checkCodeReviews(e *env.Environment, reviews []github.Review) error {
	// External code reviews should never hit this path, if they do, fail and
	// return an error.
	author := e.Author
	v, ok := r.c.CodeReviewers[author]
	if !ok {
		v, ok = r.c.DocsReviewers[author]
		if !ok {
			return trace.BadParameter("rejecting checking external review")
		}
	}

	team := v.Team

	// Teams do their own internal reviews
	switch e.Repository {
	case teleportRepo:
		team = coreTeam
	case cloudRepo:
		team = cloudTeam
	default:
		return trace.Wrap(fmt.Errorf("unsupported repository: %s", e.Repository))
	}

	setA, setB := getReviewerSets(author, team, r.c.CodeReviewers, r.c.CodeReviewersOmit)

	// PRs can be approved if you either have multiple code owners that approve
	// or code owner and code reviewer.
	if checkN(setA, reviews) >= 2 {
		return nil
	}
	if check(setA, reviews) && check(setB, reviews) {
		return nil
	}

	return trace.BadParameter("at least one approval required from each set %v %v", setA, setB)
}

func getReviewerSets(author string, team string, reviewers map[string]Reviewer, reviewersOmit map[string]bool) ([]string, []string) {
	var setA []string
	var setB []string

	for k, v := range reviewers {
		// Only assign within a team.
		if v.Team != team {
			continue
		}
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

const (
	// Commented is a code review where the reviewer has left comments only.
	Commented = "COMMENTED"
	// Approved is a code review where the reviewer has approved changes.
	Approved = "APPROVED"
	// ChangesRequested is a code review where the reviewer has requested changes.
	ChangesRequested = "CHANGES_REQUESTED"

	// Repo slugs
	cloudRepo    = "cloud"
	teleportRepo = "teleport"

	coreTeam     = "Core"
	cloudTeam    = "Cloud"
	internalTeam = "Internal"
)
