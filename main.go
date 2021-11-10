package main

import (
	"bytes"
	"context"
	"encoding/json"
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

var redisClient *redis.Client
var githubowner = "aykay76"
var githubrepo = "autograf"
var githubpat = ""
var grafanabaseurl = ""
var grafanaapitoken = ""

func main() {
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
					templateName := ns.Labels["grafana-dashboard-name"]

					td := DashboardTemplateData{ns.ObjectMeta.Labels["grafana-dashboard-name"], ns.ObjectMeta.Name}

					if len(templateName) > 0 {
						createDashboardFromTemplate(templateName, td)
					}
				} else {
					fmt.Println(err)
				}

				redisClient.XAck(stream, consumersGroup, messageID)
			}
		}
	}
}

func createDashboardFromTemplate(templateName string, td DashboardTemplateData) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubpat},
	)
	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)

	templatePath := filepath.Join("templates", templateName+".json")
	fmt.Println("Attempting to get template", templatePath, "from", githubowner, "/", githubrepo)
	reader, response, err := githubClient.Repositories.DownloadContents(context.TODO(), githubowner, githubrepo, templatePath, nil)
	fmt.Println(response.Response.Status)
	fmt.Println(response.Response.Header)
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

	// Also upload to Grafana
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
	c.CreateDashboard(dashboard)
}
