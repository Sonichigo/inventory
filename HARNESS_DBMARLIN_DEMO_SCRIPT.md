## INTRODUCTION

### Presenter's Introduction
### What is Harness Database DevOps?
### What is DBmarlin?

---


## PRE-DEMO SETUP (do this before the audience joins)

The following pipelines must be available in the Harness project:
- Reset Demo: resets the database to a known good state and records a performance baseline
- Deploy New Release: deploys the new release that will introduce a performance regression

DBmarlin must be connected to the target PostgreSQL database and integrated with Harness so that pipeline events appear as markers on the DBmarlin timeline.
The CV health check must be configured to query DBmarlin wait time metrics after deployment.
Execute Reset Demo first and confirm CV checks are green before proceeding.
Execute Deploy New Release – MJ approximately 20 minutes before the main demo begins, so that the performance degradation is already visible in the DBmarlin timeline when the audience is watching.


---


## DEMO WALKTHROUGH


### Step 1 — Reset the environment and establish a baseline

Harness is in use for database deployments as part of the overall application development and deployment process.
Show the Reset Demo pipeline in Harness. This pipeline resets the state of the database and records a baseline of reasonable performance. The green trace in the pipeline execution view shows the total time taken after deploying the reset.
After Reset Demo has completed, CV checks confirm that things are healthy.
Switch to DBmarlin and set the time range to Last Hour with Play selected so that the view updates in real time. Hovering over the schema change markers on the DBmarlin timeline should show 3 functional indexes being added during the reset. This is the healthy baseline that we will compare against after the bad deployment.


### Step 2 — Deploy the new release

Twenty minutes before the live demo, execute the Deploy New Release – MJ pipeline. This pipeline pushes out the new release and deploys database changes that will cause query performance to worsen.
The pipeline is integrated with DBmarlin to ensure that new deployments do not affect performance. Once the database changes are deployed, Continuous Verification (CV) queries DBmarlin to measure wait time — that is, how long the application has to wait for queries to complete.
When the demo starts with the audience, the application is running as it would before the deployment of the new release. Harness is waiting in the CV step.
DBmarlin's timeline chart shows load (orange line) and wait time (green bars) over the time period. There is a Harness event marker showing that Harness has notified DBmarlin of the pipeline execution, as well as detected schema changes introduced by the pipeline.


### Step 3 — CV catches the regression

Continuous Verification shows that there is a problem affecting database performance.
The CV step has picked up the degraded database performance from DBmarlin. DBmarlin detected this within minutes of the deployment. The pipeline has automatically failed and paused — no manual monitoring was needed.
The DBmarlin timeline confirms the degradation, and the event view shows both the notification from Harness and the detection of the schema change — indexes being dropped — which is the likely cause of the performance problem.
The demo now moves to DBmarlin to diagnose the issue and plan remediation steps.


### Step 4 — Diagnose the problem in DBmarlin

Go into DBmarlin and open the Statements view. The timeline showed degradation, and events showed the Harness notification and the index drop. Now we need to identify exactly which query is affected and why.
Select the top statement and open the execution plan timeline.
DBmarlin shows that the execution plan has worsened considerably, and that this coincides precisely with the Harness pipeline execution and the detected schema change. The cost has jumped from 5.06 to 1,741.42 — a dramatic regression caused by PostgreSQL switching from an index scan to a sequential scan after the indexes were dropped.


### Step 5 — Ask DBmarlin Co-pilot for a fix

Select the slower execution plan and open DBmarlin Co-pilot (AI). Ask Co-pilot to explain the problem and recommend a fix.
DBmarlin Co-pilot has identified the root cause and produced specific remediation steps. The recommended SQL is:
```sql
    CREATE INDEX idx_lower_location ON suppliers(LOWER(location));
    CREATE INDEX idx_lower_item ON suppliers(LOWER(item));
```
These two functional indexes will allow PostgreSQL to use index scans instead of sequential scans on the suppliers table, restoring the original query cost.
The index creation statements can now be copied and taken directly into Harness to be packaged as a governed database change.


### Step 6 — Author the fix in Harness

Open Harness AI and go to the Author DB Change screen. Enter the following prompt using the SQL recommended by DBmarlin Co-pilot:
```sql
    "DBmarlin has identified a significant performance issue, and has made the following recommendations:
    CREATE INDEX idx_lower_location ON suppliers(LOWER(location));
    CREATE INDEX idx_lower_item ON suppliers(LOWER(item));
    Please package this up as a database change in yaml format."
```
Harness AI has produced a valid, governed Liquibase changelog. Note the correct process governance: the change is identified, the change author is recorded, and change labels are attached. This is not just automation — it is automation with full audit trail.


### Step 7 — Approve and deploy the fix

For completeness, manually approve the workflow to proceed with the deployment.
Show the approval gate in Harness, then allow the pipeline to complete. The two indexes are deployed to the database.
Switch back to DBmarlin. The timeline shows performance returning to baseline as the indexes take effect and PostgreSQL begins using index scans again.


---

## What did we just show?

Harness was able to coordinate and deploy complex database changes as part of a fully automated CI/CD pipeline.
That extra speed comes with better governance — in this case, performance control. The CV process showed that the database changes had an adverse effect on performance and stopped the pipeline automatically, preventing the bad deployment from reaching production undetected.
DBmarlin was able to diagnose the problem quickly, identify the root cause at the execution plan level, and recommend an exact fix using AI Co-pilot — all within minutes of the deployment.
Harness was able to package and deploy that fix, still with full control of the database change environment, governance intact, and an audit trail from the moment the issue was detected to the moment it was resolved.
Harness and DBmarlin allow organisations to innovate faster, and at the same time reduce the risk of change.
