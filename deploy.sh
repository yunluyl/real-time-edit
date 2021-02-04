#!/bin/bash -e
docker build -t gcr.io/yunlu-test/realtime-server:latest .
docker push gcr.io/yunlu-test/realtime-server:latest
gcloud compute instance-groups managed rolling-action replace real-time-server-group-us-central-1 --max-surge=0 --max-unavailable=3 --replacement-method=recreate --region=us-central1 --project=yunlu-test