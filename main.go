package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"text/template"

	"github.com/aykay76/gtemplater/pkg/grafapi"

	"github.com/go-redis/redis"
	"github.com/google/go-github/v39/github"
	"github.com/rs/xid"
	"golang.org/x/oauth2"
	v1 "k8s.io/api/core/v1"
)

// Data structure that contains the elements that can be templated into the dashboard
type DashboardTemplateData struct {
	Title     string
	Namespace string
}

var (
	githubowner        = *flag.String("github-owner", "", "Owner of the repository in GitHub")
	githubrepo         = *flag.String("github-repo", "", "GitHub repository where templates and dashboards are stored")
	githubbranch       = *flag.String("github-branch", "master", "Name of branch where dashboards will get created")
	githubtemplatepath = *flag.String("github-template-path", "/templates", "The path in the repo where templates will be stored")
	githubpat          = *flag.String("github-pat", "", "Personal access token for GitHub")
	grafanaapitoken    = *flag.String("grafana-api-token", "eyXxxxxxx", "The REST API token for your Grafana server")
	grafanabaseurl     = *flag.String("grafana-base-url", "http://localhost:3000", "The home page URL you use to access Grafana")
	help               = flag.Bool("help", false, "do you need help with the command line?")
	redisClient        *redis.Client
)

func envOverride(key string, value string) string {
	temp, set := os.LookupEnv(key)
	if set {
		fmt.Println("Overriding", key, "from environment variable")
		return temp
	}

	return value
}

func main() {
	flag.CommandLine.Parse(os.Args[1:])

	grafanaapitoken = envOverride("GRAFANA_API_TOKEN", grafanaapitoken)
	grafanabaseurl = envOverride("GRAFANA_BASE_URL", grafanabaseurl)
	githubtemplatepath = envOverride("GITHUB_TEMPLATE_PATH", githubtemplatepath)
	githubbranch = envOverride("GITHUB_BRANCH", githubbranch)
	githubowner = envOverride("GITHUB_OWNER", githubowner)
	githubpat = envOverride("GITHUB_ACCESS_TOKEN", githubpat)
	githubrepo = envOverride("GITHUB_REPOSITORY", githubrepo)

	var ro redis.Options
	ro.Addr = os.Getenv("REDIS_ADDR")

	redisClient = redis.NewClient(&ro)
	_, err := redisClient.Ping().Result()
	if err != nil {
		log.Fatal("Unable to connect to Redis, cannot proceed", err)
	}
	log.Println("Connected to Redis server")

	stream := "kubernetes"
	consumersGroup := "kubernetes-consumer-group"
	err = redisClient.XGroupCreate(stream, consumersGroup, "0").Err()
	if err != nil {
		log.Println(err)
	}

	// generate a new reader group
	uniqueID := xid.New().String()
	for {
		entries, err := redisClient.XReadGroup(&redis.XReadGroupArgs{
			Group:    consumersGroup,
			Consumer: uniqueID,
			Streams:  []string{stream, ">"},
			Count:    1,
			Block:    0,
			NoAck:    false,
		}).Result()
		if err != nil {
			log.Fatal(err)
		}

		for i := 0; i < len(entries[0].Messages); i++ {
			messageID := entries[0].Messages[i].ID
			values := entries[0].Messages[i].Values
			eventDescription := fmt.Sprintf("%v", values["whatHappened"])
			nsJson := fmt.Sprintf("%v", values["k8sObject"])

			fmt.Println(nsJson)

			if eventDescription == "namespace added" {
				var ns v1.Namespace
				err = json.Unmarshal([]byte(nsJson), &ns)

				if err == nil {
					templateName := ns.Labels["grafana-template"]

					if len(templateName) > 0 {
						if createDashboardFromTemplate(templateName, ns.Labels["grafana-dashboard-name"], ns) == true {
							// only ACK the message on success
							redisClient.XAck(stream, consumersGroup, messageID)
						}
					}
				} else {
					fmt.Println(err)
				}
			}
		}
	}
}

func createDashboardFromTemplate(templateName string, targetName string, td interface{}) bool {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubpat},
	)
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)

	templatePath := filepath.Join(githubtemplatepath, templateName+".json")
	fmt.Println("Attempting to get template", templatePath, "from", githubowner, "/", githubrepo)
	reader, githubResponse, err := githubClient.Repositories.DownloadContents(context.TODO(), githubowner, githubrepo, templatePath, nil)
	fmt.Println(githubResponse.Response.Status)
	if githubResponse.Response.StatusCode >= 400 {
		return false
	}

	content, err := ioutil.ReadAll(reader)
	if err != nil {
		fmt.Println(err)
	}
	reader.Close()

	// substitute the templated variables
	t, err := template.New("dashboard").Parse(string(content))
	if err != nil {
		fmt.Println(err)
	}

	var b bytes.Buffer
	err = t.Execute(&b, td)
	if err != nil {
		fmt.Println(err)
	}

	content = b.Bytes()

	// Upload to Grafana
	c := grafapi.NewClient(grafanabaseurl, grafanaapitoken)
	var dashboardContent interface{}
	json.Unmarshal(content, &dashboardContent)
	dashboard := grafapi.Dashboard{
		Content:   dashboardContent,
		FolderId:  0,
		FolderUid: "",
		Message:   "Creating dashboard from Autograf",
		Overwrite: true,
	}

	// TODO: get return information and store in git repo as state for future changes
	//       or using the uid get the full dashboard definition and store in git
	fmt.Println("Creating dashboard in Grafana...")
	grafanaResponse, dashboardResponse := c.CreateDashboard(dashboard)
	if grafanaResponse.StatusCode >= 200 && grafanaResponse.StatusCode < 300 {
		fmt.Println("Getting full dashboard from Grafana...")
		dashboardContent := c.GetDashboard("/api/dashboards/uid/" + dashboardResponse.Uid)

		// dashboard was added successfully, state changed so publish an event
		payloadBytes, err := json.Marshal(dashboardContent)
		if err == nil {
			publishMessage("dashboard created", targetName+".json", payloadBytes)
		} else {
			fmt.Println(err)
		}
	} else {
		fmt.Println("Status from Grafana does not indicate success", grafanaResponse.Status)
	}

	return true
}

func publishMessage(whatHappened string, filename string, payload []byte) error {
	fmt.Println("Publishing to Redis..", whatHappened)
	fmt.Println(string(payload))

	return redisClient.XAdd(&redis.XAddArgs{
		Stream:       "dashboards",
		MaxLen:       0,
		MaxLenApprox: 0,
		ID:           "",
		Values: map[string]interface{}{
			"whatHappened": whatHappened,
			"filename":     filename,
			"payload":      payload,
		},
	}).Err()
}
