// Copyright © 2017 Paul Sanford <me@paulsanford.net>
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

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Runs full suite of checks against PRs",
	Long: `This commands retrieves all open PRs and the last 20 closed ones. It checks for three things:
* If the PR has a JIRA ticket number in the title somewhere
* If the associated JIRA tickets have release versions
* If the associated JIRA tickets are in sprints

Active PRs get a long-form display with ticket names, release versions, and sprints spelled out.
Closed PRs get one line with the PR title, whether the tickets have a sprint or release associated.
For closed PRs, the lines will display a green s for active sprint, yellow c for closed sprint, or red n
for no sprint. They will also display a green r for release version or red n for no release version.

You can specify 'closed' to only check closed PRs and 'open' to only check open PRs.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !viper.IsSet("github-username") || !viper.IsSet("github-token") || !viper.IsSet("organization") || !viper.IsSet("repos") {
			fmt.Println("github config info needed")
			fmt.Println("github-username, github-token, organization, repos (list)")
			return
		}
		if !viper.IsSet("jira-username") || !viper.IsSet("jira-password") || !viper.IsSet("jira-url") || !viper.IsSet("jira-sprint-field") {
			fmt.Println("jira info is needed")
			fmt.Println("jira-username, jira-password, and jira-url, jira-sprint-field")
			return
		}
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: viper.GetString("github-token")},
		)
		tc := oauth2.NewClient(ctx, ts)

		green := color.New(color.FgGreen).SprintFunc()
		red := color.New(color.FgRed).SprintFunc()
		yellow := color.New(color.FgYellow).SprintFunc()

		client := github.NewClient(tc)
		jclient, err := jira.NewClient(nil, viper.GetString("jira-url"))
		jclient.Authentication.SetBasicAuth(viper.GetString("jira-username"), viper.GetString("jira-password"))
		if err != nil {
			fmt.Printf("Couldn't get JIRA client: %s\n", err)
			return
		}

		closed := true
		open := true
		if len(args) > 0 {
			if args[0] == "closed" {
				open = false
			}
			if args[0] == "open" {
				closed = false
			}
		}

		repos := viper.GetStringSlice("repos")
		for _, repo := range repos {
			if open {
				openPRs, err := getOpenPRs(client, &ctx, repo)
				if err != nil {
					fmt.Printf("Couldn't get %s open PRs: %s\n", repo, err)
					continue
				}
				for _, PR := range openPRs {
					pr := *PR
					fmt.Printf("%s\n", *pr.Title)
					tickets := findJiraTickets(*pr.Title)
					ticketArray, err := GetTickets(jclient, tickets)
					if err != nil {
						fmt.Printf("\t\tError getting tickets: %s\n", err)
						continue
					}
					for _, ticket := range ticketArray {
						fmt.Printf("\tTicket: %s\n", ticket.Number)
						fmt.Printf("\t\tTitle: %s\n", ticket.Issue.Fields.Summary)
						colorfunc := red
						if ticket.HasReleaseVersion() {
							colorfunc = green
						}
						fmt.Printf("\t\tRelease Version: %+v\n", colorfunc(ticket.Issue.Fields.FixVersions))
						sprintName := "NONE"
						colorfunc = red
						if ticket.HasSprint() {
							sprintName = ticket.CurrentSprint.Name
							colorfunc = green
						}
						fmt.Printf("\t\tCurrent Sprint: %s\n", colorfunc(sprintName))
					}
				}
			}
			if closed {
				closedPRs, err := getClosedPRs(client, &ctx, repo)
				if err != nil {
					fmt.Printf("Couldn't get %s closed PRs: %s\n", repo, err)
					continue
				}
				for _, PR := range closedPRs {
					pr := *PR
					tickets := findJiraTickets(*pr.Title)
					ticketArray, err := GetTickets(jclient, tickets)
					if err != nil {
						fmt.Printf("Error getting tickets for %s: %s\n", *pr.Title, err)
					}
					sprintFunc := green
					sprintLetter := "s"
					releaseFunc := green
					releaseLetter := "r"
					for _, ticket := range ticketArray {
						if !ticket.HasSprint() {
							if len(ticket.Sprints) > 0 {
								sprintFunc = yellow
								sprintLetter = "c"
							} else {
								sprintFunc = red
								sprintLetter = "n"
							}
						}
						if !ticket.HasReleaseVersion() {
							releaseFunc = red
							releaseLetter = "n"
						}
					}
					if len(ticketArray) > 0 {
						fmt.Printf("%d: %s %s %s\n", *pr.Number, sprintFunc(sprintLetter), releaseFunc(releaseLetter), *pr.Title)
					} else {
						fmt.Printf("%d: %s %s\n", *pr.Number, red("---"), *pr.Title)
					}
				}
			}
		}
	},
}

func GetTickets(jclient *jira.Client, numbers []string) ([]Ticket, error) {
	var tickets []Ticket
	for _, number := range numbers {
		tck := Ticket{Number: number}
		issue, _, err := jclient.Issue.Get(number, nil)
		if err != nil {
			return nil, err
		}
		tck.Issue = *issue
		sprints, err := issue.Fields.Unknowns.Array(viper.GetString("jira-sprint-field"))
		if err != nil {
			return nil, err
		}
		for _, spr := range sprints {
			sprint, _ := ParseSprint(spr.(string))
			if sprint.State == "ACTIVE" {
				tck.CurrentSprint = &sprint
			}
			tck.Sprints = append(tck.Sprints, sprint)
		}
		tickets = append(tickets, tck)
	}
	return tickets, nil
}

type Ticket struct {
	Number        string
	Issue         jira.Issue
	Sprints       []Sprint
	CurrentSprint *Sprint
}

func (t Ticket) HasSprint() bool {
	return t.CurrentSprint != nil
}

func (t Ticket) HasReleaseVersion() bool {
	return len(t.Issue.Fields.FixVersions) > 0
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
	return getPRs(client, ctx, repo, opt)
}

func getClosedPRs(client *github.Client, ctx *context.Context, repo string) ([]*github.PullRequest, error) {
	opt := &github.PullRequestListOptions{
		State:     "closed",
		Sort:      "updated",
		Direction: "desc",
	}
	opt.PerPage = closedNum
	return getPRs(client, ctx, repo, opt)
}

func getPRs(client *github.Client, ctx *context.Context, repo string, opt *github.PullRequestListOptions) ([]*github.PullRequest, error) {
	PRs, _, err := client.PullRequests.List(*ctx, viper.GetString("organization"), repo, opt)
	if err != nil {
		return nil, err
	}
	return PRs, nil
}

var closedNum int

func init() {
	RootCmd.AddCommand(checkCmd)

	checkCmd.Flags().IntVarP(&closedNum, "closed_number", "c", 10, "Number of closed PRs to retrieve per repo")
}
