# 감사 — SDD 무인 3주기 run-04: THREE-CYCLE-PASS

2026-09-11. 판정: **THREE-CYCLE-PASS.** 3회차 전부 완료. 네 번째 시도에서 조건을 충족했다.

판정은 그 세션의 보고를 그대로 받아 적은 것이 아니라 아래를 직접 확인한 결과다.

## 1. 실행 조건

| | |
|---|---|
| 세션 UUID | `2c4ceab9-a60a-4bfb-b075-6a6181a1618a` (피어 이름 `run-04-a8`) |
| 실행 명령 | `D:\AIDEV\Clauduct\clauduct.cmd --model sol --effort high` (사용자 지정) |
| launcher가 넘긴 값 | `model=gpt-5.6-sol`, `effort=high`, contextPolicy 400000/320000. `--verify-auto-compact` 미사용 |
| 작업 루트 | `verification/dev-sandbox/run-04` |
| 전달 | `2026-09-11-session-27-sdd-three-cycle-run-04.md`(비공개) 전문 1회. 원본과 문자 단위 대조해 9633자 동일 확인 |
| 관측 구간 | transcript `2026-09-11T06:38:26Z` ~ `09:15:34Z` (2시간 37분) |

## 2. PASS 조건 — 직접 확인한 증거

**1) 3개 플랜이 실제 테스트와 로컬 커밋까지 완료됐다.**

`git -C run-04 log`로 확인한 커밋 3개. 작성자는 시험용 identity다.

| 커밋 | 시각 | 제목 |
|---|---|---|
| `b1639f9` | 16:48 | feat: add JSONL stats command |
| `e2cd785` | 17:31 | feat: add log filtering and tail commands |
| `8ff78e3` | 18:13 | feat: add report and export commands |

`git status --short --untracked-files=all` 비어 있음. 추적 파일은 `README.md`, `VERIFICATION.md`, `cli.mjs`, `logs.mjs`, `package.json`, `test/logs.test.mjs`.

**러너를 직접 다시 돌려 확인했다** — 그 세션의 보고와 독립적인 실행이다.

```
pwsh -NoProfile -NonInteractive -File src/run-node-tests.ps1 -Root .../run-04
→ tests 63, pass 63, fail 0, cancelled 0, skipped 0, todo 0, exit 0
```

재실행 후에도 작업 트리는 clean이다.

**2) 회차마다 구현 자식 1명과 검토 자식 2명이 실제로 실행됐다.**

transcript의 `Agent` 도구 호출 **9건**, 플랜당 3건이다 — 구현 / 명세 검토 / 품질 검토. 검토는 순차였다.

검토가 형식적이지 않았다는 증거가 남아 있다. 검토 자식들이 구체적 결함을 반환했고 그것이 수정 회차로 이어졌다.

- 플랜 2 명세 검토 `acb06e14`: "판정은 SPEC PASS가 아니라 concrete defect 목록"
- 플랜 2 품질 검토 `ac0966ad`: "QUALITY PASS 불가. HIGH: 일반 객체 `COMMAND_OPTIONS`"
- 그 결과가 `VERIFICATION.md`의 수정 1 RED(`__proto__`가 command allowlist를 통과) → GREEN으로 기록돼 있다.
- 플랜 1 품질 검토 `a736f0f5`는 High 후보를 냈다가 스스로 "중요 정정"으로 철회했다. 검토가 실제로 코드를 읽었다는 방향의 증거다.

**3) 최초 지시 이후 개입이 없었다.**

transcript에 들어온 외부 메시지는 `clauduct-ba`(이 세션)가 보낸 **최초 프롬프트 1건뿐**이다. 나머지 inbound는 전부 자기 자식의 `agent-message`와 `task-notification`이다. 사용자 입력도, 이 세션의 회신도 없다.

## 3. Not verified로 남는 것

- **자식의 effort.** 9명 전부 미확인이다.
- **자식의 model.** 9명 중 8명은 자식 호스트의 `SubagentStart Environment`에서 `gpt-5.6-luna`로 관측됐다. 플랜 1 품질 검토 1명은 관측되지 않았다. 요청값(`sonnet`)과 실제 라우팅 사이의 내부 매핑은 관측 대상이 아니었으므로 여전히 Not verified다.
- **메인 세션의 실제 model·effort.** launcher가 넘긴 인수는 확인했으나 세션 내부에서 그 값이 쓰였다는 관측은 없다.
- **종료 JSON.** 세션이 종료된 시점에 launcher가 출력한 `CLAUDUCT_REQUEST_STATUS <JSON>`을 아직 확보하지 못했다. `cleanup` 9개·`admission`·registration eviction·`unsupportedEventNames`·`unknownBetaNames`·스트리밍 fallback 분기·`webSearch*`는 그 JSON이 있어야 채워진다. 이 실행으로 **채울 수 있는** 값이지 채워진 값이 아니다.

## 4. 기록된 실제 장애 1건

플랜 3 명세 검토 자식이 `SPEC PASS` 근거를 메시지로 반환한 뒤, 그 자식의 최종 completion transport가 API `server_error` timeout이었다. 메인은 결과가 이미 있으므로 재실행하지 않았다. 같은 단위를 두 번 돌리지 않는다는 규칙대로다.

호스트가 띄운 알림은 이렇다.

```
<status>failed</status>
<summary>Agent "플랜 3 명세 검토" failed: Agent terminated early due to
an API error: Request timed out (error type server_error)</summary>
```

알림은 "resume 할 수 있다"고 안내했고 메인은 하지 않았다. 결과가 이미 메시지로 와 있었고 같은 단위를 두 번 돌리지 않는 규칙이 우선한다. 실패 알림에서의 올바른 비-재실행이 실측된 것이다.

이것은 합성이 아닌 실제 upstream 실패 관측이며, 종료 JSON을 확보하면 스트리밍 오류 행의 증거가 될 수 있다. 종료 JSON 없이는 이 실패가 스트리밍 단계였는지 분류할 수 없다.

## 5. 네 번의 시도

| | run-01 | run-02 | run-03 | run-04 |
|---|---|---|---|---|
| 거부한 guard | `test-node-runner-cap` | `test-node-runner-cap` | `bash-nested-shell` | 없음 |
| 거부 후 행동 | 계속 진행 | 계속 진행 | 즉시 중단 | — |
| 자식 | 없음 | 없음 | 없음 | 9명 |
| 판정 | 절차 불합격 | 절차 불합격 | SAFE-STOP 0/3 | **THREE-CYCLE-PASS 3/3** |

run-03에서 고친 두 가지가 그대로 작동했다. `VERIFICATION.md`가 회차마다 "`TEST_RUNNER_FAILED`나 guard 거부는 없었다"를 명시하고 있다 — 최초 확인 명령을 프롬프트가 직접 적은 것과, 빈 루트를 `TEST_FILE_NOT_FOUND`로 이름 붙인 러너 수정이 각각의 함정을 없앴다.

## 6. 이 통과가 증명하지 않는 것

SDD 3회 연속 개발 능력의 증거다. 무제한·수시간 운영 안정성, 중단·복구, 모든 모델·Workflow, 보안 격리 전체, 자동 압축 기본값 발동은 증명하지 않는다. 자식 라우팅은 model 8/9 관측, effort 0/9다.
