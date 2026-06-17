# File Size Budget Hotspots

- Created at: 2026-06-17T18:46:40.7619417Z
- Scope: handwritten Go/Markdown/PowerShell/Bash file-size budget snapshot; not a code-quality score
- Files checked: 1175
- Warnings: 0
- Failures: 0
- Hotspots at >= 75% of warning threshold: 0

| File | Kind | Lines | Warn | Max | Warn % | Max % |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| docs\sdd\message-service.md | docs | 864 | 1200 | 1500 | 72 | 57.6 |
| docs\sdd\contacts-service.md | docs | 794 | 1200 | 1500 | 66.2 | 52.9 |
| loadtest\demo\run-local-secure-demo.ps1 | script/runner | 734 | 1000 | 1500 | 73.4 | 48.9 |
| docs\runbook\local-loadtest.md | docs | 686 | 1200 | 1500 | 57.2 | 45.7 |
| tools\run-loadtest-capacity-baseline-suite.ps1 | script/runner | 633 | 1000 | 1500 | 63.3 | 42.2 |
| docs\sdd\push-gateway.md | docs | 602 | 1200 | 1500 | 50.2 | 40.1 |
| docs\architecture\target-architecture-platform.md | docs | 574 | 1200 | 1500 | 47.8 | 38.3 |
| services\identity-service\cmd\identity-service\main_test.go | test/runner | 1137 | 2500 | 3000 | 45.5 | 37.9 |
| docs\architecture\target-architecture-ai.md | docs | 546 | 1200 | 1500 | 45.5 | 36.4 |
| services\contacts-service\internal\infrastructure\postgres\repository_test.go | test/runner | 1089 | 2500 | 3000 | 43.6 | 36.3 |
| docs\architecture\target-architecture-timeline.md | docs | 542 | 1200 | 1500 | 45.2 | 36.1 |
| loadtest\contacts\main.go | test/runner | 1057 | 2500 | 3000 | 42.3 | 35.2 |
| docs\runbook\loadtest\message-service\loadtest-report-20260609.md | docs | 528 | 1200 | 1500 | 44 | 35.2 |
| loadtest\pushgateway\run-local-smoke.ps1 | script/runner | 524 | 1000 | 1500 | 52.4 | 34.9 |
| services\push-gateway\internal\infrastructure\redisroute\registry_test.go | test/runner | 1047 | 2500 | 3000 | 41.9 | 34.9 |
| services\policy-service\internal\infrastructure\postgres\evaluator_test.go | test/runner | 1042 | 2500 | 3000 | 41.7 | 34.7 |
| loadtest\sendmessage\main.go | test/runner | 1024 | 2500 | 3000 | 41 | 34.1 |
| docs\sdd\delivery-service.md | docs | 510 | 1200 | 1500 | 42.5 | 34 |
| services\message-service\internal\infrastructure\postgres\repository_mutation_test.go | test/runner | 978 | 2500 | 3000 | 39.1 | 32.6 |
| services\api-gateway\internal\api\grpc\server_test.go | test/runner | 975 | 2500 | 3000 | 39 | 32.5 |

This is a complexity governance snapshot only. Large files are review priorities, not automatic design failures.
