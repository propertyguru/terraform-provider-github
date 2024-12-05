/*
The code in this file is written to suit the needs in PropertyGuru use case.
  - The functions are named with "pg" prefix, indicating PropertyGuru.
  - There are small modifications as well in the other files, mostly to replace the invocation of original function with
    the new function in this file.
  - Any modification to the original code is commented with [PG]......
  - If you want to search what's being modified and why it is being modified, you can search [PG] in the codebase.
    Alternatively, you can also diff the commits.
*/
package github

import (
	"context"
	"net/http"
	"sync"

	"github.com/google/go-github/v66/github"
)

type PGGetAllTeamsResponse struct {
	teams []*github.Team
	resp  *github.Response
	err   error
}

type PGGetAllTeamReposResponse struct {
	repos []*github.Repository
	resp  *github.Response
	err   error
}

// Constants
var pgGithubOrgName string = "propertyguru"
var pgGithubOrgId int64 = 1661612

// This contains the response of API call to get all the teams
var pgGetAllTeamsResponse PGGetAllTeamsResponse

// This contains the response of API call to get all the repositories in a team
var pgGetAllTeamReposResponse PGGetAllTeamReposResponse

// This is the map of team ID or team slug to the team object
// We just use normal Map, not a Sync Map, because the writes to it are only done by one thread, and the other threads
// waits until the write operation complete. Thus, no race condition possible.
var pgTeamsByTeamId = make(map[int64]*github.Team)
var pgTeamsByTeamSlug = make(map[string]*github.Team)

// This is the map of team ID and repository name to the repository object
// A nested map where the first key is the team ID and the second key is the repository name
// Using sync.Map because the writes to it are done by many threads, and the reads can happen between those writes
var pgReposByTeamIdAndRepoName sync.Map

// Mutexes
var pgMutexInitializeLocalDataTeams sync.Mutex
var pgDoneGetAllTeams bool = false
var pgMutexInitializeLocalDataTeamRepos = make(map[int64]*sync.Mutex)
var pgDoneGetAllTeamRepos = make(map[int64]bool)

/**********************/
/* INTERNAL FUNCTIONS */
/**********************/
// The functions below should be only called from this file only
// as part of the operations in getting the resources in bulk from calling GitHub API

func pgInitializeLocalDataTeams(ctx context.Context, client *github.Client) error {
	// Let only one thread get all teams from GitHub
	// If one thread is already starting to get all teams, let the other threads wait until the work is done
	pgMutexInitializeLocalDataTeams.Lock()
	if !pgDoneGetAllTeams {
		page := 1
		for {
			opts := github.ListOptions{PerPage: 100, Page: page}
			teams, resp, err := client.Teams.ListTeams(ctx, pgGithubOrgName, &opts)
			pgGetAllTeamsResponse = PGGetAllTeamsResponse{teams, resp, err}

			if pgGetAllTeamsResponse.err != nil {
				return pgGetAllTeamsResponse.err
			}

			for _, team := range pgGetAllTeamsResponse.teams {
				pgTeamsByTeamId[team.GetID()] = team
				pgTeamsByTeamSlug[team.GetSlug()] = team
				pgMutexInitializeLocalDataTeamRepos[team.GetID()] = &sync.Mutex{}
				pgDoneGetAllTeamRepos[team.GetID()] = false
			}

			if len(pgGetAllTeamsResponse.teams) < 100 {
				break
			}
			page++
		}
		pgDoneGetAllTeams = true
	}
	pgMutexInitializeLocalDataTeams.Unlock()
	return pgGetAllTeamsResponse.err
}

func pgInitializeLocalDataTeamRepos(ctx context.Context, client *github.Client, teamID int64) error {
	// Let only one thread get all repositories in a team from GitHub
	// If one thread is already starting to get all repositories in a team, let the other threads wait until the work is done
	pgMutexInitializeLocalDataTeamRepos[teamID].Lock()
	if !pgDoneGetAllTeamRepos[teamID] {
		mapRepoNameToRepo := make(map[string]*github.Repository)
		page := 1
		for {
			opts := github.ListOptions{PerPage: 100, Page: page}
			repos, resp, err := client.Teams.ListTeamReposByID(ctx, pgGithubOrgId, teamID, &opts)
			pgGetAllTeamReposResponse = PGGetAllTeamReposResponse{repos, resp, err}

			if pgGetAllTeamReposResponse.err != nil {
				return pgGetAllTeamReposResponse.err
			}

			for _, repo := range pgGetAllTeamReposResponse.repos {
				mapRepoNameToRepo[repo.GetName()] = repo
			}

			if len(pgGetAllTeamReposResponse.repos) < 100 {
				break
			}
			page++
		}
		pgReposByTeamIdAndRepoName.Store(teamID, mapRepoNameToRepo)
		pgDoneGetAllTeamRepos[teamID] = true
	}
	pgMutexInitializeLocalDataTeamRepos[teamID].Unlock()
	return pgGetAllTeamReposResponse.err
}

/**********************/
/* EXTERNAL FUNCTIONS */
/**********************/
// The functions below are to be called from other files
// For example, "pgGetTeamByTeamId" is called from file resource_github_team.go

func pgGetTeamByTeamId(ctx context.Context, client *github.Client, id int64) (*github.Team, *github.Response, error) {
	pgInitializeLocalDataTeams(ctx, client)
	if team, teamFound := pgTeamsByTeamId[id]; teamFound {
		return team, pgGetAllTeamsResponse.resp, pgGetAllTeamsResponse.err
	}
	err := pgGetAllTeamsResponse.err
	if err == nil {
		err = &github.ErrorResponse{Response: &http.Response{StatusCode: http.StatusNotFound}}
	}
	return nil, pgGetAllTeamsResponse.resp, err
}

func pgGetTeamByTeamSlug(ctx context.Context, client *github.Client, slug string) (*github.Team, *github.Response, error) {
	pgInitializeLocalDataTeams(ctx, client)
	if team, teamFound := pgTeamsByTeamSlug[slug]; teamFound {
		return team, pgGetAllTeamsResponse.resp, pgGetAllTeamsResponse.err
	}
	err := pgGetAllTeamsResponse.err
	if err == nil {
		err = &github.ErrorResponse{Response: &http.Response{StatusCode: http.StatusNotFound}}
	}
	return nil, pgGetAllTeamsResponse.resp, err
}

func pgGetRepoByTeamIDAndRepoName(ctx context.Context, client *github.Client, teamID int64, repoName string) (*github.Repository, *github.Response, error) {
	pgInitializeLocalDataTeamRepos(ctx, client, teamID)
	if mapRepoNameToRepo, ok := pgReposByTeamIdAndRepoName.Load(teamID); ok {
		if repo, repoFound := mapRepoNameToRepo.(map[string]*github.Repository)[repoName]; repoFound {
			return repo, pgGetAllTeamReposResponse.resp, pgGetAllTeamReposResponse.err
		}
	}
	err := pgGetAllTeamReposResponse.err
	if err == nil {
		err = &github.ErrorResponse{Response: &http.Response{StatusCode: http.StatusNotFound}}
	}
	return nil, pgGetAllTeamReposResponse.resp, err
}
