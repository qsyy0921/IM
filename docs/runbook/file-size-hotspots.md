# File Size Budget Hotspots

- Created at: 2026-06-18T16:26:48.5551571Z
- Scope: handwritten Go/Markdown/PowerShell/Bash file-size budget snapshot; not a code-quality score
- Files checked: 1236
- Warnings: 0
- Failures: 0
- Hotspots at >= 80% of warning threshold: 1

| File | Kind | Lines | Warn | Max | Warn % | Max % |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| docs\sdd\message-service.md | docs | 1048 | 1200 | 1500 | 87.3 | 69.9 |
| docs\sdd\contacts-service.md | docs | 945 | 1200 | 1500 | 78.8 | 63 |
| docs\runbook\local-loadtest.md | docs | 768 | 1200 | 1500 | 64 | 51.2 |
| docs\sdd\push-gateway.md | docs | 743 | 1200 | 1500 | 61.9 | 49.5 |
| loadtest\demo\run-local-secure-demo.ps1 | script/runner | 734 | 1000 | 1500 | 73.4 | 48.9 |
| docs\architecture\target-architecture-ai.md | docs | 687 | 1200 | 1500 | 57.2 | 45.8 |
| tools\run-loadtest-capacity-baseline-suite.ps1 | script/runner | 669 | 1000 | 1500 | 66.9 | 44.6 |
| services\identity-service\cmd\identity-service\main_test.go | test/runner | 1272 | 2500 | 3000 | 50.9 | 42.4 |
| loadtest\contacts\main.go | test/runner | 1246 | 2500 | 3000 | 49.8 | 41.5 |
| docs\runbook\loadtest\message-service\loadtest-report-20260609.md | docs | 622 | 1200 | 1500 | 51.8 | 41.5 |

This is a complexity governance snapshot only. Large files are review priorities, not automatic design failures.
