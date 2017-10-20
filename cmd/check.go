// Copyright © 2017 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	// For github oauth library
	_ "golang.org/x/oauth2/github"

	"github.com/andygrunwald/go-jira"
	"github.com/fatih/color"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

// checkCmd represents the check command
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !viper.IsSet("github-username") || !viper.IsSet("github-token") || !viper.IsSet("organization") || !viper.IsSet("repos") {
			fmt.Println("github config info needed")
			return
		}
		if !viper.IsSet("jira-username") || !viper.IsSet("jira-password") {
			fmt.Println("jira info is needed")
			return
		}
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: viper.GetString("github-token")},
		)
		tc := oauth2.NewClient(ctx, ts)

		green := color.New(color.FgGreen).SprintFunc()
		red := color.New(color.FgRed).SprintFunc()

		client := github.NewClient(tc)
		jclient, err := jira.NewClient(nil, "https://upguard.atlassian.net")
		jclient.Authentication.SetBasicAuth(viper.GetString("jira-username"), viper.GetString("jira-password"))
		if err != nil {
			fmt.Printf("Couldn't get JIRA client: %s\n", err)
			return
		}

		repos := viper.GetStringSlice("repos")
		for _, repo := range repos {
			goPRs, err := getOpenPRs(client, &ctx, repo)
			if err != nil {
				fmt.Printf("Couldn't get %s PRs: %s\n", repo, err)
				return
			}
			for _, PR := range goPRs {
				pr := *PR
				fmt.Printf("%s\n", *pr.Title)
				tickets := findJiraTickets(*pr.Title)
				fmt.Printf("\tTickets: %+v\n", tickets)
				for _, ticket := range tickets {
					issue, _, err := jclient.Issue.Get(ticket, nil)
					if err != nil {
						fmt.Printf("\t\tError getting issue %s: %s\n", ticket, err)
					} else {
						fmt.Printf("\t\tTitle: %s\n", issue.Fields.Summary)
						colorfunc := red
						if len(issue.Fields.FixVersions) > 0 {
							colorfunc = green
						}
						fmt.Printf("\t\tRelease Version: %+v\n", colorfunc(issue.Fields.FixVersions))
						sprints, _ := issue.Fields.Unknowns.Array("customfield_10006")
						var currentSprint *Sprint
						if len(sprints) > 0 {
							for k := range sprints {
								newspr, _ := ParseSprint(sprints[k].(string))
								if newspr.State == "ACTIVE" {
									currentSprint = &newspr
									break
								}
							}
						}
						sprintName := "NONE"
						colorfunc = red
						if currentSprint != nil {
							sprintName = currentSprint.Name
							colorfunc = green
						}
						fmt.Printf("\t\tCurrent Sprint: %s\n", colorfunc(sprintName))
					}
				}
			}
		}
	},
}

func ParseSprint(input string) (Sprint, error) {
	idre := regexp.MustCompile("id=([0-9]+),")
	idstrings := idre.FindStringSubmatch(input)
	spr := Sprint{ID: 0, Name: "", State: ""}
	spr.ID, _ = strconv.Atoi(idstrings[1])
	namre := regexp.MustCompile("name=([^,]+),")
	namestrings := namre.FindStringSubmatch(input)
	spr.Name = namestrings[1]
	statere := regexp.MustCompile("state=([A-Z]+),")
	statestrings := statere.FindStringSubmatch(input)
	spr.State = statestrings[1]
	return spr, nil
}

type Sprint struct {
	ID    int
	Name  string
	State string
}

func findJiraTickets(PRTitle string) []string {
	re := regexp.MustCompile("[A-Z]{3,4}[- ][0-9]+")
	tickets := re.FindAllString(PRTitle, -1)
	hre := regexp.MustCompile(" ")
	for i, ticket := range tickets {
		tickets[i] = hre.ReplaceAllString(ticket, "-")
	}
	return tickets
}

func getOpenPRs(client *github.Client, ctx *context.Context, repo string) ([]*github.PullRequest, error) {
	opt := &github.PullRequestListOptions{
		State: "open",
	}
	opt.PerPage = 100
	PRs, _, err := client.PullRequests.List(*ctx, viper.GetString("organization"), repo, opt)
	if err != nil {
		return nil, err
	}
	return PRs, nil
}

func init() {
	RootCmd.AddCommand(checkCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// checkCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// checkCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
