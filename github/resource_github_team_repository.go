package github

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/google/go-github/v62/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceGithubTeamRepository() *schema.Resource {
	return &schema.Resource{
		Create: resourceGithubTeamRepositoryCreate,
		Read:   resourceGithubTeamRepositoryRead,
		Update: resourceGithubTeamRepositoryUpdate,
		Delete: resourceGithubTeamRepositoryDelete,
		Importer: &schema.ResourceImporter{
			State: func(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				teamIdString, username, err := parseTwoPartID(d.Id(), "team_id", "username")
				if err != nil {
					return nil, err
				}

				teamId, err := getTeamID(teamIdString, meta)
				if err != nil {
					return nil, err
				}

				d.SetId(buildTwoPartID(strconv.FormatInt(teamId, 10), username))
				return []*schema.ResourceData{d}, nil
			},
		},

		Schema: map[string]*schema.Schema{
			"team_id": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "ID or slug of team",
			},
			"repository": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The repository to add to the team.",
			},
			"permission": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     "pull",
				Description: "The permissions of team members regarding the repository. Must be one of 'pull', 'triage', 'push', 'maintain', 'admin' or the name of an existing custom repository role within the organisation.",
			},
			"etag": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceGithubTeamRepositoryCreate(d *schema.ResourceData, meta interface{}) error {
	err := checkOrganization(meta)
	if err != nil {
		return err
	}

	client := meta.(*Owner).v3client
	orgId := meta.(*Owner).id

	// The given team id could be an id or a slug
	givenTeamId := d.Get("team_id").(string)
	teamId, err := getTeamID(givenTeamId, meta)
	if err != nil {
		return err
	}

	orgName := meta.(*Owner).name
	repoName := d.Get("repository").(string)
	permission := d.Get("permission").(string)
	ctx := context.Background()

	_, err = client.Teams.AddTeamRepoByID(ctx,
		orgId,
		teamId,
		orgName,
		repoName,
		&github.TeamAddTeamRepoOptions{
			Permission: permission,
		},
	)

	if err != nil {
		return err
	}

	d.SetId(buildTwoPartID(strconv.FormatInt(teamId, 10), repoName))

	// [PG] Unnecessary to call the API again after creating the team repository
	// return resourceGithubTeamRepositoryRead(d, meta)
	return nil
}

func resourceGithubTeamRepositoryRead(d *schema.ResourceData, meta interface{}) error {
	err := checkOrganization(meta)
	if err != nil {
		return err
	}

	client := meta.(*Owner).v3client
	// orgId := meta.(*Owner).id

	teamIdString, repoName, err := parseTwoPartID(d.Id(), "team_id", "repository")
	if err != nil {
		return err
	}
	teamId, err := getTeamID(teamIdString, meta)
	if err != nil {
		return err
	}
	// orgName := meta.(*Owner).name
	ctx := context.WithValue(context.Background(), ctxId, d.Id())
	if !d.IsNewResource() {
		ctx = context.WithValue(ctx, ctxEtag, d.Get("etag").(string))
	}

	// [PG] Use the customized function to get team repos
	// repo, resp, repoErr := client.Teams.IsTeamRepoByID(ctx, orgId, teamId, orgName, repoName)
	repo, resp, repoErr := pgGetRepoByTeamIDAndRepoName(ctx, client, teamId, repoName)
	if repoErr != nil {
		if ghErr, ok := repoErr.(*github.ErrorResponse); ok {
			if ghErr.Response.StatusCode == http.StatusNotModified {
				return nil
			}
			if ghErr.Response.StatusCode == http.StatusNotFound {
				log.Printf("[INFO] Removing team repository association %s from state because it no longer exists in GitHub",
					d.Id())
				d.SetId("")
				return nil
			}
		}
		return err
	}

	if err = d.Set("etag", resp.Header.Get("ETag")); err != nil {
		return err
	}
	if d.Get("team_id") == "" {
		// If team_id is empty, that means we are importing the resource.
		// Set the team_id to be the id of the team.
		if err = d.Set("team_id", teamIdString); err != nil {
			return err
		}
	}
	if err = d.Set("repository", repo.GetName()); err != nil {
		return err
	}
	if err = d.Set("permission", getPermission(repo.GetRoleName())); err != nil {
		return err
	}

	return nil
}

func resourceGithubTeamRepositoryUpdate(d *schema.ResourceData, meta interface{}) error {
	err := checkOrganization(meta)
	if err != nil {
		return err
	}

	client := meta.(*Owner).v3client
	orgId := meta.(*Owner).id

	teamIdString, repoName, err := parseTwoPartID(d.Id(), "team_id", "repository")
	if err != nil {
		return err
	}
	teamId, err := strconv.ParseInt(teamIdString, 10, 64)
	if err != nil {
		return unconvertibleIdErr(teamIdString, err)
	}
	orgName := meta.(*Owner).name
	permission := d.Get("permission").(string)
	ctx := context.WithValue(context.Background(), ctxId, d.Id())

	// the go-github library's AddTeamRepo method uses the add/update endpoint from GitHub API
	_, err = client.Teams.AddTeamRepoByID(ctx,
		orgId,
		teamId,
		orgName,
		repoName,
		&github.TeamAddTeamRepoOptions{
			Permission: permission,
		},
	)

	if err != nil {
		return err
	}
	d.SetId(buildTwoPartID(teamIdString, repoName))

	return resourceGithubTeamRepositoryRead(d, meta)
}

func resourceGithubTeamRepositoryDelete(d *schema.ResourceData, meta interface{}) error {
	err := checkOrganization(meta)
	if err != nil {
		return err
	}

	client := meta.(*Owner).v3client
	orgId := meta.(*Owner).id

	teamIdString, repoName, err := parseTwoPartID(d.Id(), "team_id", "repository")
	if err != nil {
		return err
	}
	teamId, err := strconv.ParseInt(teamIdString, 10, 64)
	if err != nil {
		return unconvertibleIdErr(teamIdString, err)
	}
	orgName := meta.(*Owner).name
	ctx := context.WithValue(context.Background(), ctxId, d.Id())

	resp, err := client.Teams.RemoveTeamRepoByID(ctx, orgId, teamId, orgName, repoName)

	if resp.Response.StatusCode == 404 {
		log.Printf("[DEBUG] Failed to find team %s to delete for repo: %s.", teamIdString, repoName)
		repo, _, err := client.Repositories.Get(ctx, orgName, repoName)
		if err != nil {
			return err
		}
		newRepoName := repo.GetName()
		if newRepoName != repoName {
			log.Printf("[INFO] Repo name has changed %s -> %s. "+
				"Try deleting team repository again.",
				repoName, newRepoName)
			_, err := client.Teams.RemoveTeamRepoByID(ctx, orgId, teamId, orgName, newRepoName)
			return err
		}
	}

	return err
}
