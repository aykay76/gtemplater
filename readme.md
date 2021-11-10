# gtemplater

A basic microservice that creates Grafana dashboards from a template stored in a GitHub repository. Where the templates are stored is completely configurable through the following command line flags or environment variables (environment variables are default, command line flags override for local testing):

|Environment variable|Command line flag|Meaning
|-|-|-|
|`AUTOGRAF_MODE`|mode as first command line option|The mode of operation for Autograf. Currently supported modes are `monitor` and `reconcile`|
|`AUTOGRAF_INSIDE`|--inside[=true]|Autograf is running inside k8s if true, and authenticates with service account. Autograf is running outside k8s if false, and authenticates with kubeconfig file|
|`GITHUB_ACCESS_TOKEN`|--github-pat|Personal access token used to access GitHub repository where templates and dashboards are stored|
|`GITHUB_OWNER`|--github-owner|The owner of the repository in GitHub, this will be an organisation or individual user account|
|`GITHUB_REPOSITORY`|--github-repo|Repository where Autograf should look for templates and dashboards|
|`GITHUB_USER`|--github-user|Full name of the user that will add dashboards to the repository|
|`GITHUB_EMAIL`|--github-email|E-mail address for commits where dashboards are added to the repository|
|`GITHUB_BRANCH`|--github-branch|The branch to add dashboards to, in case you want to approve the change first|
|`GITHUB_TEMPLATE_PATH`|--github-template-path|The template path within the above repository|
|`GITHUB_DASHBOARD_PATH`|--github-dashboard-path|The directory in the repository where dashboards will be saved|
|`GRAFANA_BASE_URL`|--grafana-base-url|The home page of Grafana that you would access in your browser (e.g. http://localhost:3000)
|`GRAFANA_API_TOKEN`|--grafana-apitoken|The API token to use for calling the Grafana REST API|

This repo follows on from my `kwatch` repo that publishes changes to k8s as events on a Redis stream. This microservice subscribes to the same topic/stream and responds by downloading a template Grafana dashboard from GitHub and executing the template (via Golang template package) using the namespace definition as the input template data.

When the dashboard has been submitted to Grafana succesfully, this service downloads the full dashboard and publishes a message on another stream `dashboards` to notify any other interested parties that there's a new dashboard in town. Currently it's my `gitupdater` microservice that simply updates a GitHub repository with the new dashboard; but it could equally be an e-mail notification microservice or a Webhook, a Teams notifiers, a Slack notifier... the whole point of microservices and EDA, who knows what in the future?