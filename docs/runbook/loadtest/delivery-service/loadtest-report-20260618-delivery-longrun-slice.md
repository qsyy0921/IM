# NexusIM Capacity Long-Run Campaign Report

- Campaign: capacity-longrun-nine-services-20260618-plan
- Status: completed
- Plan: H:\NexusIM\loadtest-results\capacity-longrun-nine-services-20260618-plan\capacity-longrun-campaign-plan.json
- Summary: H:\NexusIM\loadtest-results\capacity-longrun-nine-services-20260618-plan\capacity-longrun-campaign-summary.json
- Services completed: 1/1 selected services
- Plan services: 9
- Minimum required duration: 1800 seconds
- Minimum observed duration: 1800.189
- Boundary: local long-run campaign evidence only; not a production SLO, benchmark guarantee, or sizing proof.

| Service | Runner | Mode | Status | Duration seconds | Primary metric | Summary path | Reason |
| --- | --- | --- | --- | --- | --- | --- | --- |
| delivery-service | delivery | seeded | passed | 1800.189 | items_per_second=709.397 | H:\NexusIM\loadtest-results\capacity-longrun-nine-services-20260618-plan\delivery-service\delivery-summary.json | items_per_second=709.396987105862 |

Completed status means every selected service produced a capacity_summary with duration at least the configured minimum and a positive success or throughput metric. It is still not a production sizing claim.
