#!/bin/bash
# Update Harness Monitored Service with Custom Health Source
# Replace HARNESS_TOKEN with your actual token from the browser request

ACCOUNT_ID="nwHBifh8T2GfffuOx6ORmA"
TOKEN="eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJhdXRoVG9rZW4iOiI2OWNiNGQ0NjY4NDBiZjBjNWMyYmU2YWMiLCJpc3MiOiJIYXJuZXNzIEluYyIsImV4cCI6MTc3NTAzNjgwMiwiZW52IjoiZ2F0ZXdheSIsImlhdCI6MTc3NDk1MDM0Mn0.ytVGOdr4ShgS_MpdbL5Ydl7Ccob0CPaCgSq6Wz_auk8"

curl -s -X PUT \
  "https://app.harness.io/gateway/cv/api/monitored-service/deploy_app_dsfd?routingId=${ACCOUNT_ID}&accountId=${ACCOUNT_ID}&projectIdentifier=DBMarlin&orgIdentifier=default" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
  "orgIdentifier": "default",
  "projectIdentifier": "DBMarlin",
  "identifier": "deploy_app_dsfd",
  "name": "deploy_app_dsfd",
  "type": "Application",
  "description": "BBQBookkeeper DBMarlin Demo Monitored Service",
  "serviceRef": "deploy_app",
  "environmentRef": "dsfd",
  "tags": {},
  "sources": {
    "healthSources": [
      {
        "identifier": "bbq_dbmarlin_metrics",
        "name": "BBQ DBMarlin Metrics",
        "type": "CustomHealthMetric",
        "spec": {
          "connectorRef": "bbq_app_metrics",
          "metricDefinitions": [
            {
              "identifier": "waittime",
              "metricName": "waittime",
              "groupName": "DBMarlinActivity",
              "queryType": "SERVICE_BASED",
              "requestMethod": "GET",
              "urlPath": "dbmarlin-metrics?from=start_time&to=end_time",
              "startTimeInfo": {
                "placeholder": "start_time",
                "timestampFormat": "MILLISECONDS"
              },
              "endTimeInfo": {
                "placeholder": "end_time",
                "timestampFormat": "MILLISECONDS"
              },
              "metricResponseMapping": {
                "metricValueJsonPath": "$.[*].waittime",
                "timestampJsonPath": "$.[*].waittime",
                "serviceInstanceJsonPath": "$.[*].executions"
              },
              "analysis": {
                "riskProfile": {
                  "category": "PERFORMANCE",
                  "metricType": "RESP_TIME",
                  "thresholdTypes": ["ACT_WHEN_HIGHER"]
                },
                "deploymentVerification": {
                  "enabled": true,
                  "serviceInstanceMetricPath": "$.[*].executions"
                }
              },
              "sli": {
                "enabled": false
              }
            }
          ]
        }
      }
    ],
    "changeSources": []
  },
  "dependencies": [],
  "notificationRuleRefs": [],
  "enabled": true
}' | python3 -m json.tool