# S42 후속 수정·실제 TUI 검수 — 2026-09-20

S42에서 실패한 Workflow 거부 후 일반 대화 회복 경계를 수정했다. 역할 발견 안내, Workflow 옵션 계약, 압축 요약 지침, status의 관측 의미도 보완했다. 전체 기능 무결점 판정은 하지 않는다. 이 보고서는 아래 빌드와 실제 수행한 시나리오에만 적용한다.

## 빌드와 보존 범위

- 제품 commit: `78fcc5c9aa7f7b76036d89810998af138093adc1`. 선행 수정: `632bb6e6fba1db7e4ffb119679d4858ef037831a`.
- source: `D:\AIDEV\clauduct-s36-build\context-fork-repair-20260919`.
- Go 1.27.1, Windows amd64, CGO_ENABLED=0, `vcs.modified=false`. [build.json](build.json).
- `clauduct.exe` SHA-256: `420e7fb3e6d6d56ed128be5807efe48d6000359afb6d48812af3317f22ead2ca`.
- 테스트 대상은 개발용 `D:\AIDEV\clauduct-s36-build`의 세 바이너리다. 설치된 release와 기존 S42 실행 기록은 변경하지 않았다.
- 검증 후 같은 세 바이너리를 개발용 경로에 반영하고 hash를 재대조했다. [delivery.json](delivery.json). 기존 세 파일과 사람용 launcher는 `D:\AIDEV\clauduct-s36-build\before-78fcc5c-20260920`에 보존했다. 사람용 launcher의 빌드 검증 manifest도 이번 근거로 연결했다.
- 실제 TUI는 이미 승인된 공개 fixture project/profile을 재사용했다. 사용자 원본 S42 profile을 새 시험에 사용하지 않았다.
- raw transcript의 추론·인증 값·압축 ticket은 보고서에 복사하지 않는다. [tui-audit.json](tui-audit.json)은 식별자·행 번호·시간·선택·숫자 결과·hash만 파생한다.

## 1. Workflow 거부 뒤 회복

S42는 잘못된 회수 요청을 막았지만 해당 모델 응답 전체를 HTTP 400으로 끝냈다. native 대화 이력에 정상적인 도구 호출/거부 결과 쌍이 남지 않았고, 다음 일반 프롬프트에서도 회수 호출이 생성되어 다시 거부됐다. 두 번째 호출의 원래 인자 전체는 S42에 남아 있지 않아 모델 또는 native 하나만을 단독 원인으로 지목하지 않는다.

현재 경로는 `WORKFLOW_RECOVERY_UNVERIFIED`에 한정해 원래 실행을 **고정된 거부 script**로 교체하고, native `PreToolUse`가 `permissionDecision:deny`와 이유를 반환한다. 원래 script나 모르는 run은 실행하지 않는다. hook이 적용되지 않아도 대체 script는 오류만 발생시킨다. 거부된 호출의 원래 이력은 session/call/digest가 맞을 때만 되돌려, 다음 모델 요청에 호출과 실제 도구 오류가 함께 남는다.

이는 [native PreToolUse의 도구 거부 응답](https://code.claude.com/docs/en/hooks#pretooluse-decision-control)을 사용한다. 제한된 정상/적대 검사와 실제 native TUI에서 거부 이유의 전달을 확인했다. 검증 자체를 풀거나 API 오류를 숨기는 방식이 아니다. `rejectedWorkflowCalls`와 기능별 `rejectedToolCalls`가 거부를 별도로 남긴다. 다른 검증 오류 전체를 이 경로로 바꾸지는 않았다.

128개의 기존 Workflow origin 저장 한계는 유지한다. 그 한계를 초과하면 이력 보존을 추측하지 않고 기존 오류로 중단할 수 있다. 무제한 거부·무제한 이력은 지원 범위가 아니다.

## 2. 실제 역할·Workflow 계약

설치 native `2.1.278`에서 Agent는 필요 시 발견되며, 발견한 뒤 `agent_listing_delta`로 역할 목록이 추가되는 경로를 확인했다. 첫 검증 세션의 중간 자식은 ToolSearch 후 실제 Plan leaf를 만들었다. S42에서 역할을 발견하기 전에 Plan이 없다고 판단한 상황을 근거 없이 native의 영구 제한으로 취급하지 않는다.

ToolSearch 설명에는 역할 부재를 선언하기 전에 Agent를 발견하고 갱신된 역할 목록을 읽으라는 안내를 추가했다. 역할의 도구 제한과 native schema는 유지한다. 이 안내는 모델의 모든 판단을 결정적으로 강제하는 기능은 아니다.

Workflow의 `agent()`에 `tools`·`maxTurns`를 전달하는 기존 fixture는 인자의 전달만 증명했다. 설치 native 런타임에서 이 두 옵션의 강제 적용을 확인하지 못했다. 이제 wrapper는 두 필드가 있으면 해당 자식을 시작하기 전에 오류를 낸다. `null`도 우회가 되지 않는다. 모델 설명도 같은 제한을 알린다. 프롬프트의 “도구를 쓰지 말라”를 강제 도구 제한으로 주장하지 않는다. native의 별도 `disallowedTools` 경로 전체가 불가능하다는 결론도 아니다.

지원되는 model/effort/label/schema 옵션으로 Workflow 네 병렬 자식과 후속 합산 자식을 실행했다. 이는 [native Workflow](https://code.claude.com/docs/en/workflows)의 현재 호출 계약에 맞춘 실증이다. 임의 JavaScript, custom agentType, 중단된 Workflow 전체 재실행까지 검증한 것은 아니다.

## 3. 압축 지연과 보존

압축 요청에만 developer 지침을 추가했다. 반복된 설명·중복 보고서를 줄이고 1,200단어를 목표로 하되, 명시적 보존 요구·정확한 데이터·제약·Agent/run ID·실패·다음 작업을 우선한다. 강제 절단과 output token cap은 넣지 않았다. 확정 모델·effort를 낮추지 않았다. 지침을 포함한 **동일한 backend payload**로 사전 계수와 생성을 수행한다.

최종 TUI의 압축 두 건:

| 항목 | 첫 수동 압축 | Terra 선택 직후 수동 압축 |
|---|---:|---:|
| 실제 실행 | Sol/high | 기존 Sol/high |
| native 요청 모델 | Sol | Terra |
| 사전 계수 = backend input | 23,766 | 21,840 |
| 정확 계수 시간 | 1,324ms | 1,343ms |
| gateway 요청 전체 시간 | 81,494ms | 59,503ms |
| native 저장 요약 길이 | 7,366자 | 9,273자 |

두 번째 모델 선택 뒤 일반 프롬프트 없이 `/compact`를 보냈다. 이후 일반 생성은 Terra/medium으로 전환됐다. 압축 전 기준 미만 일반 모델 전환에서는 자동 압축이 추가되지 않았다. 첫 압축 후 같은 ID의 두 leaf를 재개했고 두 번째 압축 후에도 root marker/revision, 두 ID와 새 숫자, Workflow run ID가 유지됐다.

S42의 195,983ms·230,506ms 및 16,253자·23,491자보다 이번 표본은 작고 빨랐다. **입력이 다른 표본이므로 동일 입력 대비 개선율이나 최악 지연 상한으로 해석하지 않는다.** 두 번째 요약이 첫 번째보다 길기도 했다. 지침은 성능 선호이며 일정 시간·길이의 보장이 아니다. 239K·450K 자동 압축 경계와 모든 멀티모달 입력을 이번에 다시 시험한 것도 아니다.

## 4. 실제 TUI 근거

native `2.1.278`, 실제 구독 backend, `-p`가 아닌 대화형 TUI로 수행했다.

| 세션 / 빌드 | 수행한 검증 |
|---|---|
| `4d465e2d-370e-4f6a-a5ae-532e416ea656` / `632bb6e` | 모르는 Workflow run 거부 → 도구 없는 정상 회복(FIR-62, 43), ROOT→A→A1→Plan A2, Workflow 4→1, 완료 결과 5개 회수 |
| `16dfd222-e9b6-44a7-94d6-27f383d84850` / `78fcc5c` | 두 분기의 leaf 생성, 정상 Workflow, 모델 전환과 수동 압축 2회, 두 기존 leaf 재개, 압축 후 Workflow 결과 회수, 의도한 거부 후 일반 회복 |

첫 세션의 Workflow 원본 journal은 네 `started`가 어떤 `result`보다 먼저 있고, 네 결과 뒤에 합산 자식이 시작됨을 기록한다. 결과는 18·24·26·30·98이다. 회수 run의 `started`는 0개였다. 단순히 모델이 “재실행하지 않았다”고 답한 것만으로 판정하지 않았다.

두 번째 세션에서는 첫 압축 후 `a1eb4f6ba346dd0c4`(Sol/high)와 `ac94e0c902d03b318`(Terra/medium)를 SendMessage로 재개했다. 두 요청의 실행 구간이 겹쳤고, 실제 자식 답변은 `AMBER-17 46`, `INDIGO-28 64`였다. root도 두 본문을 받았다. 두 번 압축한 뒤 `wf_6f48df99-04f`의 완료 결과 37을 회수했고 회수 journal에 새 자식 시작이 없었다.

최종 세션은 70요청, 43회 inference, backend usage 대조 43/43 일치, API 실패 0, 결과 미확보 0, 의도한 Workflow 거부 1이었다. 첫 세션은 60요청, 대조 35/35 일치, API 실패 0, 결과 미확보 0, 의도한 거부 1이었다. 두 세션 모두 `/exit`로 종료 0, native 회수 완료, watchdog 강제 종료 없음이다. 새 제품에서 Esc 취소를 별도로 반복한 시험은 아니며 S42의 사용자 반복 취소를 자동 재시도로 재분류하지 않는다.

**추가 관측된 한계:** 두 번째 세션 초기 Astra/low 부모는 A 완료 통지보다 먼저 결과가 확보됐다고 말한 뒤 스스로 정정했다(root 44행 주장 → 46행 완료 알림 → 47행 정정). 이후 실제 두 결과와 ID는 확보됐다. transport와 결과 회수 성공이 부모의 모든 자연어 주장을 검증한 것은 아니다. `review:not_assessed_by_gateway`를 유지하며, 이 초기 설명까지 오류 없는 실행으로 포장하지 않는다. 빈 응답 대기·부모 완료 판정의 제품 통합은 여전히 별도 과제다.

## 5. status 의미

- `features.lastRequest`: 해당 기능에 집계된 완료 요청의 최대 seq. 마지막 도착 순서 때문에 작은 번호로 되돌아가지 않는다.
- `features.lastCompletedRequest`: 가장 나중에 완료 관측을 집계한 요청 seq. 동시 실행에서는 최대 seq와 다를 수 있다.
- `agentContexts.phaseMeaning:context_policy_state_not_turn_outcome`: `ready`는 압축 정책 상태이며 작업 성공 판정이 아니다.
- `completion.rejectedWorkflowCalls`: API 전송 성공과 구분되는 Workflow 도구 거부 수.
- `acceptance:not_assessed`와 `review:not_assessed_by_gateway`: 종료 0, 모델의 자기 주장, 테스트 통과만으로 작업 전체 합격을 만들지 않는다.

## 6. 실패 기록과 검사

[test-summary.json](test-summary.json): 17 packages, 1,550 test/subtest pass, fail 0, skip 3, `go vet` exit 0. 전체 검사에서 건너뛴 것은 콘솔 창 닫기, 실계정 세션, launcher 강제 종료 검사다. 별도 TUI는 위 두 세션으로 수행했고, 콘솔 종료/강제 종료는 이번 변경에서 다시 실증하지 않았다.

개발 중 실패를 보존한다:

1. 첫 거부 통합 검사 2회 실패. 대체 script에 native가 요구하는 첫 `meta` 선언이 없어 hook 전에 검증이 거절됐다. meta를 추가하고 단순 “오류 있음” 대신 **정확한 hook 거부 이유와 다음 응답**까지 검사하도록 강화했다.
2. 최초 압축 지침을 top-level instruction에 붙였을 때 정확 계수 admission이 실패했다. guard를 완화하지 않고 기존 지원 형식인 developer input entry로 옮겼다. 계수/생성 payload 일치와 일반 생성에 미적용을 검사했다.
3. 변경 없이 성공할 때까지 반복한 것이 아니다. 각각 원인에 맞는 코드 변경 후 검사를 통과했다. 이 사실을 전체 환경의 비플래키 증명으로 확대하지 않는다.

초기 실패를 포함한 raw 검사 로그는 이 폴더에 보존하고, git에는 필요한 요약·hash·메타데이터와 검증 도구를 둔다. [audit.mjs](audit.mjs), [snapshot.mjs](snapshot.mjs), [capture.mjs](capture.mjs)는 승인된 공개 테스트 경로만 다룬다. 이전 S42 근거는 [별도 보고서](../session42-analysis-20260920/REPORT.md)를 보존한다.

## 남는 경계

- 검증된 부모 대기와 빈 응답의 gateway 통합, 미완료 Workflow의 중복 실행 없는 범용 재개는 미지원 상태다.
- native `/context`의 공통 window·로컬 카테고리 추정은 바꾸지 않았다. gateway 입력 제외와 모델별 실제 정책의 근거를 대신하지 않는다.
- Workflow custom 역할·tools/maxTurns 강제 제한, 모든 멀티모달·모델 조합, 모든 Esc 시점, 모든 native 업데이트의 호환성은 이번 근거 밖이다.
- 새 입력의 backend 정확 계수 왕복과 모델 생성 지연은 남는다. 이번 변경은 지연 0 또는 모든 상황의 무결점을 보장하지 않는다.
- 기능별 지원 표는 [COMPATIBILITY.md](../../docs/v2/COMPATIBILITY.md)에 반영한다. 실패 경로의 명시적 처리와 전체 기능의 완성을 구분한다.
