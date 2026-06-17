# File Size Budget Hotspots

- Created at: 2026-06-17T08:43:07.3498708Z
- Scope: handwritten Go/Markdown/PowerShell/Bash file-size budget snapshot; not a code-quality score
- Files checked: 1136
- Warnings: 0
- Failures: 0
- Hotspots at >= 80% of warning threshold: 1

| File | Kind | Lines | Warn | Max | Warn % | Max % |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| docs\sdd\message-service.md | docs | 1048 | 1200 | 1500 | 87.3 | 69.9 |
| docs\sdd\contacts-service.md | docs | 945 | 1200 | 1500 | 78.8 | 63 |
| docs\runbook\local-loadtest.md | docs | 768 | 1200 | 1500 | 64 | 51.2 |
| docs\sdd\push-gateway.md | docs | 743 | 1200 | 1500 | 61.9 | 49.5 |
| loadtest\demo\run-local-secure-demo.ps1 | script/runner | 730 | 1000 | 1500 | 73 | 48.7 |
| docs\architecture\target-architecture-ai.md | docs | 665 | 1200 | 1500 | 55.4 | 44.3 |
| tools\run-loadtest-capacity-baseline-suite.ps1 | script/runner | 630 | 1000 | 1500 | 63 | 42 |
| docs\runbook\loadtest\message-service\loadtest-report-20260609.md | docs | 622 | 1200 | 1500 | 51.8 | 41.5 |
| docs\sdd\delivery-service.md | docs | 622 | 1200 | 1500 | 51.8 | 41.5 |
| docs\architecture\target-architecture-platform.md | docs | 576 | 1200 | 1500 | 48 | 38.4 |
| services\identity-service\cmd\identity-service\main_test.go | test/runner | 1137 | 2500 | 3000 | 45.5 | 37.9 |
| docs\runbook\history\current-goal-archive-20260611.md | docs | 563 | 1200 | 1500 | 46.9 | 37.5 |
| docs\sdd\conversation-service-member-change-saga.md | docs | 563 | 1200 | 1500 | 46.9 | 37.5 |
| services\contacts-service\internal\infrastructure\postgres\repository_test.go | test/runner | 1089 | 2500 | 3000 | 43.6 | 36.3 |
| docs\architecture\target-architecture-timeline.md | docs | 542 | 1200 | 1500 | 45.2 | 36.1 |
| docs\sdd\receipt-service.md | docs | 530 | 1200 | 1500 | 44.2 | 35.3 |
| loadtest\contacts\main.go | test/runner | 1057 | 2500 | 3000 | 42.3 | 35.2 |
| services\push-gateway\internal\infrastructure\redisroute\registry_test.go | test/runner | 1047 | 2500 | 3000 | 41.9 | 34.9 |
| services\policy-service\internal\infrastructure\postgres\evaluator_test.go | test/runner | 1042 | 2500 | 3000 | 41.7 | 34.7 |
| loadtest\pushgateway\run-local-smoke.ps1 | script/runner | 520 | 1000 | 1500 | 52 | 34.7 |

This is a complexity governance snapshot only. Large files are review priorities, not automatic design failures.
