# File Size Budget Hotspots

- Created at: 2026-06-23T00:23:55.6741184Z
- Scope: handwritten Go/Markdown/PowerShell/Bash file-size budget snapshot; not a code-quality score
- Files checked: 1992
- Warnings: 0
- Failures: 0
- Hotspots at >= 80% of warning threshold: 1

| File | Kind | Lines | Warn | Max | Warn % | Max % |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| docs\sdd\message-service.md | docs | 1048 | 1200 | 1500 | 87.3 | 69.9 |
| docs\sdd\contacts-service.md | docs | 945 | 1200 | 1500 | 78.8 | 63 |
| loadtest\agent\main.go | test/runner | 1745 | 2500 | 3000 | 69.8 | 58.2 |
| docs\architecture\target-architecture-ai.md | docs | 776 | 1200 | 1500 | 64.7 | 51.7 |
| docs\runbook\local-loadtest.md | docs | 768 | 1200 | 1500 | 64 | 51.2 |
| docs\runbook\client-platform.md | docs | 757 | 1200 | 1500 | 63.1 | 50.5 |
| docs\sdd\push-gateway.md | docs | 743 | 1200 | 1500 | 61.9 | 49.5 |
| loadtest\demo\run-local-secure-demo.ps1 | script/runner | 734 | 1000 | 1500 | 73.4 | 48.9 |
| services\api-gateway\internal\api\httpbff\server_test.go | test/runner | 1343 | 2500 | 3000 | 53.7 | 44.8 |
| tools\run-loadtest-capacity-baseline-suite.ps1 | script/runner | 669 | 1000 | 1500 | 66.9 | 44.6 |

This is a complexity governance snapshot only. Large files are review priorities, not automatic design failures.
