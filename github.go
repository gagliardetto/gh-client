package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/gagliardetto/hashsearch"
	. "github.com/gagliardetto/utilz"
	validation "github.com/go-ozzo/ozzo-validation"
	"github.com/google/go-github/github"
	"github.com/google/go-querystring/query"
)

type Client struct {
	client *github.Client
}

func NewClient(token string) *Client {
	c := &Client{}

	if token == "" {
		panic("token not provided")
	}
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	c.client = github.NewClient(tc)

	return c
}

func NewWithCustomClient(ghtcl *github.Client) *Client {
	c := &Client{}

	if ghtcl == nil {
		panic("client not provided")
	}

	c.client = ghtcl

	return c
}

var ResponseCallback func(resp *github.Response)

func onResponse(resp *github.Response) {
	if ResponseCallback != nil {
		ResponseCallback(resp)
	}
}

func isRateLimitError(err error) bool {
	_, ok := err.(*github.RateLimitError)
	return ok
}

func handleRateLimitError(err error, resp *github.Response) bool {
	if isRateLimitError(err) {
		// sleep until next reset:
		time.Sleep(resp.Rate.Reset.Sub(time.Now()))
		return true
	}
	return false
}
func IsDir(v *github.RepositoryContent) bool {
	return v.GetType() == "dir"
}

////
func (c *Client) ListReposByUser(user string) ([]*github.Repository, error) {

	client := c.client

	opt := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	// get all pages of results
	var allRepos []*github.Repository
	for {
		var repos []*github.Repository
		var resp *github.Response
		errs := RetryExponentialBackoff(5, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			repos, resp, err = client.Repositories.List(ctx, user, opt)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allRepos, nil
}
func (c *Client) ListReposByOrg(org string) ([]*github.Repository, error) {

	client := c.client

	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	// get all pages of results
	var allRepos []*github.Repository
	for {
		var repos []*github.Repository
		var resp *github.Response
		errs := RetryExponentialBackoff(5, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			repos, resp, err = client.Repositories.ListByOrg(ctx, org, opt)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allRepos, nil
}

// addOptions adds the parameters in opt as URL query parameters to s. opt
// must be a struct whose fields may contain "url" tags.
func addOptions(s string, opt interface{}) (string, error) {
	v := reflect.ValueOf(opt)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return s, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}

	qs, err := query.Values(opt)
	if err != nil {
		return s, err
	}

	u.RawQuery = qs.Encode()
	return u.String(), nil
}

func (c *Client) GetPull(owner string, repo string, number int) (*github.PullRequest, error) {
	var pull *github.PullRequest
	var resp *github.Response
	errs := RetryExponentialBackoff(5, time.Second, func() error {
		var err error

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		pull, resp, err = c.client.PullRequests.Get(ctx, owner, repo, number)
		if err != nil {
			return fmt.Errorf("error while executing request: %w", err)
		}
		onResponse(resp)
		if handleRateLimitError(err, resp) {
			return err
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
			// TODO: catch rate limit error, and wait
			return fmt.Errorf(
				"status code is: %v (%s)",
				resp.StatusCode,
				resp.Status,
			)
		}
		// nil on 200 and 404
		return nil
	})
	if errs != nil && len(errs) > 0 {
		// TODO: fix this scenario in other getters, too.
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}
		return nil, errors.New(FormatErrorArray("", errs))
	}
	if resp.StatusCode == http.StatusNotFound {
		// TODO: catch rate limit error, and wait
		return nil, ErrNotFound
	}

	return pull, nil
}

func (c *Client) ListPulls(owner string, repo string) ([]*github.PullRequest, error) {
	client := c.client

	opt := &github.ListOptions{PerPage: 100}
	// get all pages of results
	var allPRs []*github.PullRequest
	for {

		var tmpPRs []*github.PullRequest
		var resp *github.Response
		errs := RetryExponentialBackoff(5, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			tmpPRs, resp, err = client.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{State: "closed", ListOptions: *opt})
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		allPRs = append(allPRs, tmpPRs...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allPRs, nil
}

func (c *Client) GetOrg(org string) (*github.Organization, error) {
	var organization *github.Organization
	var resp *github.Response
	errs := RetryExponentialBackoff(5, time.Second, func() error {
		var err error

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		organization, resp, err = c.client.Organizations.Get(ctx, org)
		if err != nil {
			return fmt.Errorf("error while executing request: %w", err)
		}
		onResponse(resp)
		if handleRateLimitError(err, resp) {
			return err
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
			// TODO: catch rate limit error, and wait
			return fmt.Errorf(
				"status code is: %v (%s)",
				resp.StatusCode,
				resp.Status,
			)
		}
		// nil on 200 and 404
		return nil
	})
	if errs != nil && len(errs) > 0 {
		return nil, errors.New(FormatErrorArray("", errs))
	}
	if resp.StatusCode == http.StatusNotFound {
		// TODO: catch rate limit error, and wait
		return nil, ErrNotFound
	}

	return organization, nil
}

var ErrNotFound = errors.New("not found")

func (c *Client) GetUser(u string) (*github.User, error) {
	var user *github.User
	var resp *github.Response
	errs := RetryExponentialBackoff(5, time.Second, func() error {
		var err error

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		user, resp, err = c.client.Users.Get(ctx, u)
		if err != nil {
			return fmt.Errorf("error while executing request: %w", err)
		}
		onResponse(resp)
		if handleRateLimitError(err, resp) {
			return err
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
			// TODO: catch rate limit error, and wait
			return fmt.Errorf(
				"status code is: %v (%s)",
				resp.StatusCode,
				resp.Status,
			)
		}
		// nil on 200 and 404
		return nil
	})
	if errs != nil && len(errs) > 0 {
		return nil, errors.New(FormatErrorArray("", errs))
	}
	if resp.StatusCode == http.StatusNotFound {
		// TODO: catch rate limit error, and wait
		return nil, ErrNotFound
	}

	return user, nil
}

func (c *Client) GetRepo(owner, repo string) (*github.Repository, error) {
	var repository *github.Repository
	var resp *github.Response
	errs := RetryExponentialBackoff(5, time.Second, func() error {
		var err error

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		repository, resp, err = c.client.Repositories.Get(ctx, owner, repo)
		if err != nil {
			return fmt.Errorf("error while executing request: %w", err)
		}
		onResponse(resp)
		if handleRateLimitError(err, resp) {
			return err
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
			// TODO: catch rate limit error, and wait
			return fmt.Errorf(
				"status code is: %v (%s)",
				resp.StatusCode,
				resp.Status,
			)
		}
		// nil on 200 and 404
		return nil
	})
	if errs != nil && len(errs) > 0 {
		return nil, errors.New(FormatErrorArray("", errs))
	}
	if resp.StatusCode == http.StatusNotFound {
		// TODO: catch rate limit error, and wait
		return nil, ErrNotFound
	}

	return repository, nil
}

///

func (c *Client) ListOfficialMembers(org string) ([]*github.User, error) {
	client := c.client

	opt := &github.ListOptions{PerPage: 100}
	// get all pages of results
	var allUsers []*github.User
	for {
		//org.PublicMembersURL
		u := fmt.Sprintf("orgs/%v/members", org)
		u, err := addOptions(u, opt)
		if err != nil {
			return nil, err
		}
		req, err := client.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}

		var members []*github.User
		var resp *github.Response
		errs := RetryExponentialBackoff(5, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			resp, err = client.Do(ctx, req, &members)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		allUsers = append(allUsers, members...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allUsers, nil
}

///
type RepoExplorationRequest struct {
	params Params

	client *Client
}

func (a RepoExplorationRequest) Validate() error {
	return validation.ValidateStruct(&a,
		validation.Field(&a.params),
	)
}

type Params struct {
	owner, repo, path string
}

func (a Params) Validate() error {
	return validation.ValidateStruct(&a,
		validation.Field(&a.owner, validation.Required),
		validation.Field(&a.repo, validation.Required),
	)
}

func (c *Client) NewRepoExplorationRequest() *RepoExplorationRequest {
	return &RepoExplorationRequest{
		client: c,
	}
}

func (r *RepoExplorationRequest) WithOwner(owner string) *RepoExplorationRequest {
	r.params.owner = owner
	return r
}
func (r *RepoExplorationRequest) WithRepo(repo string) *RepoExplorationRequest {
	r.params.repo = repo
	return r
}
func (r *RepoExplorationRequest) WithStartPath(path string) *RepoExplorationRequest {
	r.params.path = path
	return r
}

func (r *RepoExplorationRequest) DownloadFile(filepath string) (io.ReadCloser, error) {
	err := r.Validate()
	if err != nil {
		return nil, err
	}

	r.params.path = filepath
	return r.client.client.Repositories.DownloadContents(context.Background(), r.params.owner, r.params.repo, r.params.path, nil)
}

func (r *RepoExplorationRequest) ListContents(path string) (fileContent *github.RepositoryContent, directoryContent []*github.RepositoryContent, resp *github.Response, err error) {
	err = r.Validate()
	if err != nil {
		return
	}

	r.params.path = path
	return r.client.client.Repositories.GetContents(context.Background(), r.params.owner, r.params.repo, r.params.path, nil)
}

func (r *RepoExplorationRequest) DownloadContent(v *github.RepositoryContent) (io.ReadCloser, error) {
	owner, repo, path := extractOwnerRepoPath(v)
	return r.WithOwner(owner).WithRepo(repo).DownloadFile(path)
}

func extractOwnerRepoPath(v *github.RepositoryContent) (owner, repo, path string) {
	rawurl := v.GetHTMLURL()
	htmlURL, err := url.Parse(rawurl)
	if err != nil {
		panic(err)
	}

	pathElements := strings.Split(htmlURL.Path, "/")

	owner = pathElements[1]
	repo = pathElements[2]
	path = v.GetPath()

	return
}
func (r *RepoExplorationRequest) WalkFiles(walker func(v *github.RepositoryContent) error) error {

	err := r.Validate()
	if err != nil {
		return err
	}

	// get initial contents
	_,
		directoryContent,
		resp,
		err := r.client.
		NewRepoExplorationRequest().
		WithOwner(r.params.owner).
		WithRepo(r.params.repo).
		ListContents(r.params.path)
	if err != nil {
		panic(err)
	}
	onResponse(resp)
	if handleRateLimitError(err, resp) {
		return err
	}

	return r.walkFiles(directoryContent, walker)
}

func (r *RepoExplorationRequest) walkFiles(content []*github.RepositoryContent, walker func(v *github.RepositoryContent) error) error {
	for _, v := range content {
		if IsDir(v) {
			_,
				directoryContent,
				resp,
				err := r.client.
				NewRepoExplorationRequest().
				WithOwner(r.params.owner).
				WithRepo(r.params.repo).
				ListContents(v.GetPath())
			if err != nil {
				return err
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			err = r.walkFiles(directoryContent, walker)
			if err != nil {
				return err
			}
		}

		err := walker(v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) ListOrgsOfUser(user string) ([]*github.Organization, error) {
	client := c.client

	opt := &github.ListOptions{PerPage: 100}
	// get all pages of results
	var allOrgs []*github.Organization
	for {

		var orgs []*github.Organization
		var resp *github.Response
		errs := RetryExponentialBackoff(5, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			orgs, resp, err = client.Organizations.List(ctx, user, opt)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		allOrgs = append(allOrgs, orgs...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allOrgs, nil
}

//////////////////////////////////////////
func (c *Client) ListContributors(
	owner string,
	repo string,
) ([]*github.Contributor, error) {
	client := c.client

	opt := &github.ListOptions{PerPage: 100}
	// get all pages of results
	var allContributors []*github.Contributor
	for {

		var contributors []*github.Contributor
		var resp *github.Response
		errs := RetryExponentialBackoff(5, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			contributors, resp, err = client.Repositories.ListContributors(ctx, owner, repo, &github.ListContributorsOptions{
				ListOptions: *opt,
			})
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		allContributors = append(allContributors, contributors...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allContributors, nil
}

func (c *Client) ListCommitsByAuthor(
	owner string,
	repo string,
	author string,
	maxAge time.Duration,
) ([]*github.RepositoryCommit, error) {
	return c.ListCommits(
		owner,
		repo,
		&github.CommitsListOptions{
			Author: author,
		},
		maxAge,
	)
}
func (c *Client) ListCommitsByPath(
	owner string,
	repo string,
	path string,
	maxAge time.Duration,
) ([]*github.RepositoryCommit, error) {
	return c.ListCommits(
		owner,
		repo,
		&github.CommitsListOptions{
			Path: path,
		},
		maxAge,
	)
}

func (c *Client) ListCommits(
	owner string,
	repo string,
	options *github.CommitsListOptions,
	maxAge time.Duration,
) ([]*github.RepositoryCommit, error) {
	client := c.client

	opt := &github.ListOptions{PerPage: 100}
	// get all pages of results
	var allCommits []*github.RepositoryCommit
PageLister:
	for {

		var commits []*github.RepositoryCommit
		var resp *github.Response
		errs := RetryExponentialBackoff(5, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			options.ListOptions = *opt
			commits, resp, err = client.Repositories.ListCommits(ctx, owner, repo, options)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		if maxAge > 0 {
			for _, commit := range commits {
				isTooOld := time.Now().Sub(commit.Commit.Author.GetDate()) > maxAge
				if !isTooOld {
					allCommits = append(allCommits, commit)
				} else {
					break PageLister
				}
			}
		} else {
			allCommits = append(allCommits, commits...)
		}
		if resp.NextPage == 0 {
			break PageLister
		}
		opt.Page = resp.NextPage
	}

	return allCommits, nil
}

var IsExitingFunc func() bool

func (c *Client) FindShadowMembersByContributions(
	owner string,
	repo string,
	maxAge time.Duration,
) ([]*github.Contributor, error) {

	contributors, err := c.ListContributors(owner, repo)
	if err != nil {
		return nil, fmt.Errorf("error while ListContributors: %w", err)
	}

	var shadowMembers []*github.Contributor
	for _, contributor := range contributors {
		if IsExitingFunc() {
			return shadowMembers, nil
		}
		login := contributor.GetLogin()
		commits, err := c.ListCommitsByAuthor(owner, repo, login, maxAge)
		if err != nil {
			return nil, fmt.Errorf("error while ListCommitsByAuthor for %s: %s", login, err)
		}
		isShadow := isShadowMember(commits)
		if isShadow {
			shadowMembers = append(shadowMembers, contributor)
			//debugf("	is shadow: %s", login)
		} else {
			//debugf("	not shadow: %s", login)
		}
	}

	return shadowMembers, nil
}

func isShadowMember(commits []*github.RepositoryCommit) bool {
	// direct commit: commit.author.login == commit.committer.login
	// commit was merged via the web UI: commit.committer.login == web-flow (commit.author.login is most likely the one that clicked on "Merge")
	// commit merged via a PR by another person: commit.author.login != commit.committer.login (author is the requester, the committer is the one doing the merging)

	for _, commit := range commits {
		isDirect := isDirectCommit(commit)
		if isDirect {
			return true
		}
	}
	return false
}

func isDirectCommit(commit *github.RepositoryCommit) bool {
	return commit.Author.GetLogin() == commit.Committer.GetLogin()
}
func isMergedByCommitterCommit(commit *github.RepositoryCommit) bool {
	// NOTE: isMergedByCommitter is not completely reliable because
	// I'm still not sure how to figure this out in a precise way.
	return commit.Committer.GetLogin() == "web-flow"
}
func isModeratedPRCommit(commit *github.RepositoryCommit) bool {
	return commit.Author.GetLogin() != commit.Committer.GetLogin()
}
func (c *Client) IsOwnerAnOrg(owner string) (*github.Organization, bool, error) {
	org, err := c.GetOrg(owner)
	if err != nil {
		if err == ErrNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	return org, true, nil
}
func (c *Client) IsOwnerAUser(owner string) (*github.User, bool, error) {
	user, err := c.GetUser(owner)
	if err != nil {
		if err == ErrNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	if user.GetType() == "Organization" {
		// even if the user is an Org, return the user object
		return user, false, nil
	}
	return user, true, nil
}
func (c *Client) ListLanguagesOfRepo(owner string, repo string) (map[string]int, error) {
	var languages map[string]int
	var resp *github.Response
	errs := RetryExponentialBackoff(5, time.Second, func() error {
		var err error

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		languages, resp, err = c.client.Repositories.ListLanguages(ctx, owner, repo)
		if err != nil {
			return fmt.Errorf("error while executing request: %w", err)
		}
		onResponse(resp)
		if handleRateLimitError(err, resp) {
			return err
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
			// TODO: catch rate limit error, and wait
			return fmt.Errorf(
				"status code is: %v (%s)",
				resp.StatusCode,
				resp.Status,
			)
		}
		// nil on 200 and 404
		return nil
	})
	if errs != nil && len(errs) > 0 {
		return nil, errors.New(FormatErrorArray("", errs))
	}
	if resp.StatusCode == http.StatusNotFound {
		// TODO: catch rate limit error, and wait
		return nil, ErrNotFound
	}

	return languages, nil
}
func (c *Client) ListReposBylanguage(owner string, lang string) ([]*github.Repository, error) {

	query := Sf("user:%q language:%q", owner, ToTitle(lang))

	client := c.client

	opt := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	// get all pages of results
	var allRepos []*github.Repository
	for {
		var repos *github.RepositoriesSearchResult
		var resp *github.Response
		errs := RetryExponentialBackoff(9999, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			repos, resp, err = client.Search.Repositories(ctx, query, opt)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		for repIndex := range repos.Repositories {
			allRepos = append(allRepos, &repos.Repositories[repIndex])
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allRepos, nil
}

type ListAllReposByLanguageOpts struct {
	Language     string
	ExcludeForks bool
	MinStars     int
	Limit        int
}

// Validate validates ListAllReposByLanguageOpts.
func (opts *ListAllReposByLanguageOpts) Validate() error {
	if opts == nil {
		return errors.New("opts is nil.")
	}
	if opts.Language == "" {
		return errors.New("opts.Language not provided.")
	}
	return nil
}

// ListAllReposByLanguage returns a list of (almost) all repositories
// that contain code in the specified language.
func (c *Client) ListAllReposByLanguage(opts *ListAllReposByLanguageOpts) ([]*github.Repository, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	queryFragments := make([]string, 0)
	queryFragments = append(queryFragments, Sf("language:%q", ToTitle(opts.Language)))
	if opts.ExcludeForks {
		queryFragments = append(queryFragments, "fork:false")
	}

	client := c.client

	opt := &github.SearchOptions{
		Sort:        "stars", // Sort by stargazer count.
		ListOptions: github.ListOptions{PerPage: 100},
	}
	storeIndex := hashsearch.New()

	var (
		latestStarCount int
		useStarBound    bool
		starLowerBound  int = -1 // Setting it to -1 to mean a non-written value.
	)

	// get all pages of results
	var allRepos []*github.Repository
GetterLoop:
	for {
		var repos *github.RepositoriesSearchResult
		var resp *github.Response
		errs := RetryExponentialBackoff(9999, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			query := strings.Join(queryFragments, " ")
			if useStarBound {
				withBound := append(queryFragments, Sf("stars:<=%v", starLowerBound))
				query = strings.Join(withBound, " ")
			}

			repos, resp, err = client.Search.Repositories(ctx, query, opt)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}
		for repIndex := range repos.Repositories {
			repo := &repos.Repositories[repIndex]

			if repo.GetStargazersCount() < opts.MinStars {
				break GetterLoop
			}
			id := repo.GetFullName()

			if !storeIndex.Has(id) {
				latestStarCount = repo.GetStargazersCount()

				allRepos = append(allRepos, repo)
				storeIndex.Add(id)

				if opts.Limit > 0 && len(allRepos) >= opts.Limit {
					break GetterLoop
				}
			}
		}
		if resp.NextPage == 0 {
			// If we finished all the pages (10 x 100 results = 1K repos),
			// but the starLowerBound did not get lower,
			// that means there's more than 1K repos with that specific star count,
			// but we can't get more than 1K;
			// we can only retrieve the first 1K of repos with any specific star count.

			// If starLowerBound is zero, it means we can't go any lower, and we're done:
			if starLowerBound == 0 {
				break GetterLoop
			}

			useStarBound = true
			if starLowerBound == latestStarCount {
				// For any given starLowerBound, we can only retrieve the first 1K repos.
				// For this particular starLowerBound,
				// there are more than 1K repos.

				// Let's decrement starLowerBound by one.
				// This will skip any repo with that specific star count beyond the initial 1K repos that we already got.
				latestStarCount--
			}
			starLowerBound = latestStarCount
			opt.Page = 1 // Restart
			continue GetterLoop
		}
		opt.Page = resp.NextPage
	}

	return allRepos, nil
}

type SearchReposOpts struct {
	Query    string
	MinStars int
	Limit    int
}

// Validate validates SearchReposOpts.
func (opts *SearchReposOpts) Validate() error {
	if opts == nil {
		return errors.New("opts is nil.")
	}
	if opts.Query == "" {
		return errors.New("opts.Query not provided.")
	}
	return nil
}

// SearchRepos will return a list of repos that match the provided query.
// NOTE: the repo search API does not search inside the repo contents,
// but only in its meta (repo name, description, etc.)
// To search repos by content, see `SearchCode` method.
// For more info about query syntax and parameters, see:
// https://docs.github.com/en/free-pro-team@latest/github/searching-for-information-on-github/searching-for-repositories
func (c *Client) SearchRepos(opts *SearchReposOpts) ([]*github.Repository, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	var allRepos []*github.Repository

	// Get all pages of results:
	err := c.SearchReposWithCallback(opts.Query, func(repos []*github.Repository) bool {
		for repIndex := range repos {
			repo := repos[repIndex]
			if repo.GetStargazersCount() < opts.MinStars {
				continue
			}
			allRepos = append(allRepos, repo)

			if opts.Limit > 0 && len(allRepos) >= opts.Limit {
				return false
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}

	return allRepos, nil
}

type SearchCodeOpts struct {
	Query string
	Limit int
}

// Validate validates SearchCodeOpts.
func (opts *SearchCodeOpts) Validate() error {
	if opts == nil {
		return errors.New("opts is nil.")
	}
	if opts.Query == "" {
		return errors.New("opts.Query not provided.")
	}
	return nil
}

// SearchReposWithCallback has the same functionality as SearchRepos, except the result pages are provided in a callback.
func (c *Client) SearchReposWithCallback(query string, callback func([]*github.Repository) bool) error {
	if query == "" {
		return errors.New("query not provided.")
	}

	client := c.client

	opt := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	// get all pages of results
	for {
		var repos *github.RepositoriesSearchResult
		var resp *github.Response
		errs := RetryExponentialBackoff(9999, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			repos, resp, err = client.Search.Repositories(ctx, query, opt)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return ErrNotFound
		}

		page := make([]*github.Repository, 0)
		for repIndex := range repos.Repositories {
			repo := &repos.Repositories[repIndex]
			page = append(page, repo)
		}

		doContinue := callback(page)
		if !doContinue {
			return nil
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return nil
}

// SearchCode will return a list of code results that match the provided query.
// For more info about query syntax and parameters, see:
// https://docs.github.com/en/free-pro-team@latest/github/searching-for-information-on-github/searching-code
func (c *Client) SearchCode(opts *SearchCodeOpts) ([]*github.CodeResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	client := c.client

	opt := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	// get all pages of results
	var allCodeResults []*github.CodeResult
GetterLoop:
	for {
		var repos *github.CodeSearchResult
		var resp *github.Response
		errs := RetryExponentialBackoff(9999, time.Second, func() error {
			var err error

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()

			repos, resp, err = client.Search.Code(ctx, opts.Query, opt)
			if err != nil {
				return fmt.Errorf("error while executing request: %w", err)
			}
			onResponse(resp)
			if handleRateLimitError(err, resp) {
				return err
			}

			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusNoContent {
				// TODO: catch rate limit error, and wait
				return fmt.Errorf(
					"status code is: %v (%s)",
					resp.StatusCode,
					resp.Status,
				)
			}
			// nil on 200 and 404
			return nil
		})
		if errs != nil && len(errs) > 0 {
			return nil, errors.New(FormatErrorArray("", errs))
		}
		if resp.StatusCode == http.StatusNotFound {
			// TODO: catch rate limit error, and wait
			return nil, ErrNotFound
		}

		for repIndex := range repos.CodeResults {
			allCodeResults = append(allCodeResults, &repos.CodeResults[repIndex])

			if opts.Limit > 0 && len(allCodeResults) >= opts.Limit {
				break GetterLoop
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allCodeResults, nil
}
