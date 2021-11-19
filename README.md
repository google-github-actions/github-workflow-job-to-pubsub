# GitHub Workflow Job to Pub/Sub

The GitHub Workflow Job to Pub/Sub is a small service that fulfills a GitHub
[`workflow_job` webhook][workflow-job-webhook].

-   When a job is queued, it inserts one message onto a Pub/Sub topic.

-   When a job is finished, it acknowledges one message from a Pub/Sub
    subscription.

This means that, at any time, the number of unacknowledged messages in the
Pub/Sub topic corresponds to the number of queued or active GitHub Actions jobs.
You can use this property as an indicator in an auto-scaling metric or a means
to queue new ephemeral workers.


## Deployment

This deployment example uses Google Cloud and [Cloud Run][cloud-run] to deploy
and manage the proxy.

1.  Create or use an existing Google Cloud project:

    ```sh
    export PROJECT_ID="..."
    ```

1.  Enable required APIs:

    ```sh
    gcloud services enable --project="${PROJECT_ID}" \
      artifactregistry.googleapis.com \
      cloudbuild.googleapis.com \
      pubsub.googleapis.com \
      run.googleapis.com \
      secretmanager.googleapis.com
    ```

1.  Create a service account to run the receiver:

    ```sh
    gcloud iam service-accounts create "gh-webhook-receiver" \
      --description="GitHub webhook receiver" \
      --project="${PROJECT_ID}"
    ```

1.  Create a GitHub Webhook secret and store it in Google Secret Manager. This
    secret can be any value.

    ```sh
    echo -n "<YOUR SECRET>" | gcloud secrets create "gh-webhook-secret" \
      --project="${PROJECT_ID}" \
      --data-file=-
    ```

    If you do not have a secret, you can randomly generate a secret using
    `openssl`:

    ```sh
    openssl rand -base64 32
    ```

1.  Grant the service account permissions to access the secret:

    ```sh
    gcloud secrets add-iam-policy-binding "gh-webhook-secret" \
      --project="${PROJECT_ID}" \
      --role="roles/secretmanager.secretAccessor" \
      --member="serviceAccount:gh-webhook-receiver@${PROJECT_ID}.iam.gserviceaccount.com"
    ```

1.  Create a Pub/Sub topic:

    ```sh
    gcloud pubsub topics create "gh-topic" \
      --project="${PROJECT_ID}"
    ```

1.  Grant the service account permissions to publish to the topic:

    ```sh
    gcloud pubsub topics add-iam-policy-binding "gh-topic" \
      --project="${PROJECT_ID}" \
      --role="roles/pubsub.publisher" \
      --member="serviceAccount:gh-webhook-receiver@${PROJECT_ID}.iam.gserviceaccount.com"
    ```

1.  Create a Pub/Sub subscription:

    ```sh
    gcloud pubsub subscriptions create "gh-subscription" \
      --project="${PROJECT_ID}" \
      --topic="gh-topic" \
      --ack-deadline="10" \
      --message-retention-duration="1800s" \
      --expiration-period="never" \
      --min-retry-delay="5s" \
      --max-retry-delay="30s"
    ```

1.  Grant the service account permissions to pull from the subscription:

    ```sh
    gcloud pubsub subscriptions add-iam-policy-binding "gh-subscription" \
      --project="${PROJECT_ID}" \
      --role="roles/pubsub.subscriber" \
      --member="serviceAccount:gh-webhook-receiver@${PROJECT_ID}.iam.gserviceaccount.com"
    ```

1.  Create a repository in Artifact Registry to store the container:

    ```sh
    gcloud artifacts repositories create "gh-webhook-receiver" \
      --project="${PROJECT_ID}" \
      --repository-format="docker" \
      --location="us" \
      --description="GitHub webhook receiver"
    ```

1.  Build and push the container:

    ```sh
    gcloud builds submit . \
      --project="${PROJECT_ID}" \
      --tag="us-docker.pkg.dev/${PROJECT_ID}/gh-webhook-receiver/gh-webhook-receiver"
    ```

1.  Deploy the service and attach the secret (see
    [Configuration](#configuration) for more information on available options):

    ```sh
    gcloud beta run deploy "gh-webhook-receiver" \
      --quiet \
      --project="${PROJECT_ID}" \
      --region="us-east1" \
      --set-secrets="GITHUB_WEBHOOK_SECRET=gh-webhook-secret:1" \
      --set-env-vars="PUBSUB_TOPIC_NAME=projects/${PROJECT_ID}/topics/gh-topic,PUBSUB_SUBSCRIPTION_NAME=projects/${PROJECT_ID}/subscriptions/gh-subscription" \
      --image="us-docker.pkg.dev/${PROJECT_ID}/gh-webhook-receiver/gh-webhook-receiver" \
      --service-account="gh-webhook-receiver@${PROJECT_ID}.iam.gserviceaccount.com" \
      --allow-unauthenticated
    ```

    Take note of the URL. It is important to note that this is a
    **publicly-accessible URL**.

1.  Create an organization webhook on GitHub:

    - **Payload URL:** URL for the Cloud Run service above.
    - **Content type:** application/json
    - **Secret:** value from above
    - **Events:** select "individual events" and then choose **only** "Workflow jobs"


## Configuration

-   `GITHUB_WEBHOOK_SECRET` - this is the secret key to use for authenticating
    the webhook's HMAC. This must match the value given to GitHub when
    configuring the webhook. It is very important that you choose a high-entropy
    secret, because your service must be publicly accessible.

-   `PUBSUB_TOPIC_NAME` - this is the name of the topic on which to publish.
    This must be the full topic name including the project (e.g.
    `projects/my-project/topics/my-topic`).

-   `PUBSUB_SUBSCRIPTION_NAME` - this is the name of the subscription on which
    to pull and acknowledge. This must be the full subscription name including
    the project (e.g. `projects/my-project/subscriptions/my-subscription`).


## FAQ

**Q: Why is Pub/Sub necessary? Why not just have the service create ephemeral runners or scale a VM pool directly?**
<br>
A: GitHub has a [timeout of 10 seconds][webhook-timeout] for webhook responses,
and strongly recommends asynchronous processing. Modifying an autoscaling group
or spinning up a new virtual machine will almost always exceed this timeout.

[workflow-job-webhook]: https://docs.github.com/en/developers/webhooks-and-events/webhooks/webhook-events-and-payloads#workflow_job
[webhook-timeout]: https://docs.github.com/en/rest/guides/best-practices-for-integrators#favor-asynchronous-work-over-synchronous
