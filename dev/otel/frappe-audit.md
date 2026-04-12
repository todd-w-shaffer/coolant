# Frappe theming audit

## Totals

- ⚠️ mixed: 0

- ⚪ default: 10


- 🌈 classic-palette: 4

- ✅ themed: 78

## claude-cfo.json

| id | title | status | mode | notes |
|---|---|---|---|---|
| 100 | Projected Monthly Run Rate | ✅ themed | `thresholds` | — |
| 101 | Month-to-Date Spend | ✅ themed | `thresholds` | — |
| 102 | Cost Burden per Engineer | ✅ themed | `thresholds` | — |
| 200 | Budget Burn (vs. $1000/mo hypothetical) | ✅ themed | `thresholds` | — |
| 201 | Capitalizable Output Ratio | ✅ themed | `thresholds` | — |
| 202 | FTE Equivalent (at $100/hr, 160hr/mo) | ✅ themed | `thresholds` | — |
| 203 | Active Engineers | ✅ themed | `fixed` | — |
| 300 | Daily Spend Burn Curve | ✅ themed | `fixed` | — |
| 301 | Cumulative Spend vs. Budget Line | ✅ themed | `palette-classic-by-name` | — |
| 400 | Cost by Engineer | ✅ themed | `thresholds` | — |
| 401 | Cost by Organization | 🌈 classic-palette | `palette-classic-by-name` | mode=palette-classic-by-name |
| 500 | Model Spend Mix | ✅ themed | `thresholds` | — |
| 501 | Cost per Engineer per Day | ✅ themed | `fixed` | — |
| 502 | Projected Annual Spend | ✅ themed | `thresholds` | — |

## claude-insights.json

| id | title | status | mode | notes |
|---|---|---|---|---|
| 100 | Think-to-Ship Ratio | ✅ themed | `thresholds` | — |
| 101 | Cost per Line | ✅ themed | `thresholds` | — |
| 102 | Lines per Dollar | ✅ themed | `thresholds` | — |
| 103 | Cost per Active Minute | ✅ themed | `thresholds` | — |
| 200 | Hottest Sessions (Cost Burn Rate) | ✅ themed | `thresholds` | — |
| 201 | Fastest Sessions (Lines/min) | ✅ themed | `thresholds` | — |
| 300 | Think-to-Ship Ratio Over Time | ✅ themed | `fixed` | — |
| 301 | Cache Efficiency Over Time | ✅ themed | `fixed` | — |
| 400 | Cost Rate by Session | 🌈 classic-palette | `palette-classic` | mode=palette-classic |
| 401 | Cache Savings Estimate | ✅ themed | `thresholds` | — |
| 500 | Effective Hourly Rate | ✅ themed | `thresholds` | — |
| 501 | Engineer Equivalent | ✅ themed | `thresholds` | — |
| 502 | Max Plan Multiplier | ✅ themed | `thresholds` | — |
| 510 | Code Churn Ratio | ✅ themed | `thresholds` | — |
| 511 | Net Lines Shipped | ✅ themed | `fixed` | — |
| 512 | Cache Build / Read Balance | ✅ themed | `thresholds` | — |
| 513 | Avg Session Weight | ✅ themed | `fixed` | — |
| 520 | Output Generation Rate | ✅ themed | `fixed` | — |
| 521 | Cost per Edit Decision | ✅ themed | `thresholds` | — |
| 522 | Avg Session Lifespan | ✅ themed | `fixed` | — |
| 530 | Per-Session Think-to-Ship | ✅ themed | `thresholds` | — |
| 531 | Code Churn Over Time | ✅ themed | `palette-classic-by-name` | — |
| 540 | Max Team Size | ✅ themed | `thresholds` | — |
| 541 | Concurrent Sessions Over Time | ✅ themed | `fixed` | — |

## claude-models.json

| id | title | status | mode | notes |
|---|---|---|---|---|
| 500 | Cost by Model | ✅ themed | `palette-classic-by-name` | — |
| 501 | Tokens by Model | ✅ themed | `palette-classic-by-name` | — |
| 502 | Share of Spend | ✅ themed | `palette-classic-by-name` | — |
| 510 | Cost per Line by Model | ✅ themed | `thresholds` | — |
| 511 | Tokens per Line by Model | ✅ themed | `thresholds` | — |
| 520 | Cache Hit Rate by Model | ✅ themed | `thresholds` | — |
| 521 | Output/Input Ratio by Model | ✅ themed | `thresholds` | — |
| 530 | Cost Over Time by Model | ✅ themed | `palette-classic-by-name` | — |
| 531 | Token Rate by Model | ✅ themed | `palette-classic-by-name` | — |
| 540 | Model Mix Comparison | ✅ themed | `thresholds` | — |

## claude-spend.json

| id | title | status | mode | notes |
|---|---|---|---|---|
| 1 |  | ⚪ default | `(none)` | no color block |
| 10 | List-Rate Cost | ✅ themed | `thresholds` | — |
| 11 | Tokens | ✅ themed | `fixed` | — |
| 12 | Cache Hit Rate | ✅ themed | `thresholds` | — |
| 13 | Active Time | ✅ themed | `fixed` | — |
| 14 | Sessions | ✅ themed | `fixed` | — |
| 15 | Lines Changed | ✅ themed | `fixed` | — |
| 2 |  | ⚪ default | `(none)` | no color block |
| 20 | Cost by Model | ✅ themed | `palette-classic-by-name` | — |
| 21 | Token Mix | ✅ themed | `thresholds` | — |
| 22 | Spend per Session | ✅ themed | `thresholds` | — |
| 3 |  | ⚪ default | `(none)` | no color block |
| 30 | Cost Over Time | 🌈 classic-palette | `palette-classic-by-name` | mode=palette-classic-by-name |
| 31 | Token Rate by Type | ✅ themed | `palette-classic` | — |
| 4 |  | ⚪ default | `(none)` | no color block |
| 40 | Lines Added vs Removed | ✅ themed | `fixed` | — |
| 41 | Edit Decisions | ✅ themed | `palette-classic` | — |
| 42 | Active Time | ✅ themed | `fixed` | — |

## claude-techdebt.json

| id | title | status | mode | notes |
|---|---|---|---|---|
| 100 | Cost per Line Added — by Repo | ✅ themed | `thresholds` | — |
| 101 | Cache Hit Rate — by Repo | ✅ themed | `thresholds` | — |
| 200 | Think-to-Ship Ratio — by Repo | ✅ themed | `thresholds` | — |
| 201 | Edit Rejection Rate — by Repo | ✅ themed | `thresholds` | — |
| 300 | Total Spend — by Repo | ✅ themed | `thresholds` | — |
| 301 | Cost per Line Added — Daily Trend | 🌈 classic-palette | `palette-classic` | mode=palette-classic |

## claude-vpeng.json

| id | title | status | mode | notes |
|---|---|---|---|---|
| 1 | Team Productivity | ⚪ default | `(none)` | no color block |
| 10 | Team Throughput | ✅ themed | `fixed` | — |
| 11 | Net Lines Shipped (7d) | ✅ themed | `fixed` | — |
| 12 | Team Context Efficiency | ✅ themed | `thresholds` | — |
| 2 | Adoption Signals | ⚪ default | `(none)` | no color block |
| 20 | Active Engineers (7d) | ✅ themed | `fixed` | — |
| 21 | Parallelization Gain | ✅ themed | `fixed` | — |
| 22 | Sessions per Engineer | ✅ themed | `fixed` | — |
| 23 | Agent Sprawl Index | ✅ themed | `thresholds` | — |
| 3 | Adoption Depth | ⚪ default | `(none)` | no color block |
| 30 | Engineer Adoption Depth | ✅ themed | `thresholds` | — |
| 4 | Work Character | ⚪ default | `(none)` | no color block |
| 40 | Work Profile Shift | ✅ themed | `fixed` | — |
| 41 | Team Context Efficiency Over Time | ✅ themed | `fixed` | — |
| 5 | Force Multiplier & Heavy Tails | ⚪ default | `(none)` | no color block |
| 50 | Force Multiplier (Lines per Active Hour) | ✅ themed | `fixed` | — |
| 51 | Heavy-Tail Session Audit (Top 10 by Cost) | ✅ themed | `thresholds` | — |
| 6 | Parallelization | ⚪ default | `(none)` | no color block |
| 60 | Concurrent Sessions Over Time | ✅ themed | `fixed` | — |
| 61 | Velocity by Engineer | ✅ themed | `thresholds` | — |
