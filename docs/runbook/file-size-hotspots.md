# File Size Budget Hotspots

- Created at: 2026-06-17T03:49:02.4812502Z
- Scope: handwritten Go/Markdown/PowerShell/Bash file-size budget snapshot; not a code-quality score
- Files checked: 1066
- Warnings: 0
- Failures: 0
- Hotspots at >= 80% of warning threshold: 2

| File | Kind | Lines | Warn | Max | Warn % | Max % |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| docs\sdd\message-service.md | docs | 1048 | 1200 | 1500 | 87.3 | 69.9 |
| docs\sdd\contacts-service.md | docs | 945 | 1200 | 1500 | 78.8 | 63 |
| loadtest\pushgateway\run-local-smoke.ps1 | script/runner | 897 | 1000 | 1500 | 89.7 | 59.8 |
| services\receipt-service\internal\infrastructure\postgres\repository_test.go | test/runner | 1694 | 2500 | 3000 | 67.8 | 56.5 |
| services\conversation-service\internal\infrastructure\postgres\repository_test.go | test/runner | 1564 | 2500 | 3000 | 62.6 | 52.1 |
| docs\runbook\local-loadtest.md | docs | 768 | 1200 | 1500 | 64 | 51.2 |
| docs\sdd\push-gateway.md | docs | 743 | 1200 | 1500 | 61.9 | 49.5 |
| loadtest\demo\run-local-secure-demo.ps1 | script/runner | 730 | 1000 | 1500 | 73 | 48.7 |
| services\contacts-service\internal\infrastructure\postgres\repository_test.go | test/runner | 1418 | 2500 | 3000 | 56.7 | 47.3 |
| services\receipt-service\internal\infrastructure\postgres\repository.go | production | 1650 | 2500 | 3500 | 66 | 47.1 |
| loadtest\contacts\main.go | test/runner | 1395 | 2500 | 3000 | 55.8 | 46.5 |
| loadtest\demo\main.go | test/runner | 1350 | 2500 | 3000 | 54 | 45 |
| docs\architecture\target-architecture-ai.md | docs | 665 | 1200 | 1500 | 55.4 | 44.3 |
| services\delivery-service\cmd\delivery-service\main.go | production | 1488 | 2500 | 3500 | 59.5 | 42.5 |
| loadtest\receipt\main.go | test/runner | 1270 | 2500 | 3000 | 50.8 | 42.3 |
| tools\run-loadtest-capacity-baseline-suite.ps1 | script/runner | 630 | 1000 | 1500 | 63 | 42 |
| services\identity-service\internal\infrastructure\postgres\repository.go | production | 1466 | 2500 | 3500 | 58.6 | 41.9 |
| docs\sdd\delivery-service.md | docs | 622 | 1200 | 1500 | 51.8 | 41.5 |
| docs\runbook\loadtest\message-service\loadtest-report-20260609.md | docs | 622 | 1200 | 1500 | 51.8 | 41.5 |
| services\message-service\internal\trigger\outbox\relay_test.go | test/runner | 1220 | 2500 | 3000 | 48.8 | 40.7 |

This is a complexity governance snapshot only. Large files are review priorities, not automatic design failures.
