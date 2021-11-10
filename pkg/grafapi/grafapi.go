package grafapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

type PatchDocument struct {
	Operation string      `json:"op"`
	Path      string      `json:"path"`
	Value     interface{} `json:"value"`
}

type Dashboard struct {
	Content   interface{} `json:"dashboard"`
	FolderId  int         `json:"folderId"`
	FolderUid string      `json:"folderUid"`
	Message   string      `json:"message"`
	Overwrite bool        `json:"overwrite"`
}

type DashboardResponse struct {
	Id      int    `json:"id"`
	Slug    string `json:"slug"`
	Status  string `json:"status"`
	Uid     string `json:"uid"`
	Url     string `json:"url"`
	Version int    `json:"version"`
}

// Client : a struct to encapsulate the ADO context and core client
type ApiClient struct {
	baseUri string
	headers map[string]string
}

// Connect : connect to ADO and return a client ready for acion
func NewClient(baseUri string, apiToken string) *ApiClient {
	apiClient := ApiClient{}

	apiClient.baseUri = baseUri
	apiClient.headers = make(map[string]string)
	apiClient.headers["Accept"] = "application/json"
	apiClient.headers["Authorization"] = "Bearer " + apiToken
	apiClient.headers["Content-Type"] = "application/json"

	return &apiClient
}

// ListProjects : return a page of results
func (c *ApiClient) CreateDashboard(dashboard Dashboard) DashboardResponse {
	status, responseBody := c.postObject("/api/dashboards/db", dashboard)
	fmt.Println(status)
	fmt.Println(string(responseBody))

	var response DashboardResponse
	json.Unmarshal(responseBody, &response)

	return response
}

// example response:
// {"id":2,"slug":"monitoring-the-monitor","status":"success","uid":"Ui4ofGcnz","url":"/d/Ui4ofGcnz/monitoring-the-monitor","version":1}

func (c *ApiClient) get(url string) (string, []byte) {
	var req *http.Request
	var resp *http.Response
	var err error

	req, err = http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal(err)
	}

	for k, v := range c.headers {
		req.Header.Add(k, v)
	}

	httpClient := http.DefaultClient

	resp, err = httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	return resp.Status, body
}

func (c *ApiClient) post(url string, body []byte) (string, []byte) {
	var req *http.Request
	var resp *http.Response
	var err error

	fmt.Println("Posting to", c.baseUri+url)
	fmt.Println(string(body))
	req, err = http.NewRequest(http.MethodPost, c.baseUri+url, bytes.NewBuffer(body))
	if err != nil {
		fmt.Println(err)
	}

	for k, v := range c.headers {
		req.Header.Add(k, v)
	}

	httpClient := http.DefaultClient

	resp, err = httpClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return err.Error(), nil
	}

	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	responseBody, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(responseBody))

	return resp.Status, responseBody
}

func (c *ApiClient) postObject(url string, body interface{}) (string, []byte) {
	var bodyBytes []byte
	var err error

	bodyBytes, err = json.Marshal(body)
	if err != nil {
		fmt.Println(err)
	}

	return c.post(url, bodyBytes)
}

func (c *ApiClient) patch(url string, doc []PatchDocument) (string, []byte) {
	body, _ := json.Marshal(doc)

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}

	for k, v := range c.headers {
		req.Header.Add(k, v)
	}
	// replace content-type header with patch doc
	req.Header.Del("Content-Type")
	req.Header.Add("Content-Type", "application/json-patch+json")

	httpClient := http.DefaultClient

	var resp *http.Response
	resp, err = httpClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()
	responseBody, _ := ioutil.ReadAll(resp.Body)

	fmt.Println("response Status:", resp.Status)
	if resp.StatusCode != 404 {
		fmt.Println("response Headers:", resp.Header)
		fmt.Println("response Body:", string(responseBody))
	}

	return resp.Status, responseBody
}
