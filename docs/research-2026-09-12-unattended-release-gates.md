# Clauduct 무인 연속 개발 검증 제안

현재 HOLD를 해제하려면 **장애 후 자동 재개, 인증 유지, 기본 컨텍스트 압축, 병렬 작업 격리, 장시간 자원 안정성, 개발 결과의 독립 검증**을 같은 릴리즈에서 입증해야 한다. 기존의 정상 요청·명시적 resume 성공은 출발점이며 이 조건 전체의 증거는 아니다.

유한한 시험으로 무한한 시간 동안 오류가 없음을 증명할 수는 없다. 달성 가능한 제품 목표는 총 세션 길이에 임의 종료 제한을 두지 않으면서, 지원 환경에서 사람이 개입하지 않아도 작업을 이어가고, 일시 장애를 복구하며, 결과가 불명확한 외부 효과를 중복 실행하지 않는 것이다. 이 목표를 시간·부하·복구 시간·작업 완료율로 측정해야 한다. 안전하게 멈추는 기능의 성공과 무인 작업 완주의 성공은 별도로 판정한다.

이 문서는 **검증과 개선의 제안**이다. 기존 HOLD 판정을 변경하지 않으며, 아래 신규 시험의 합격을 선언하지 않는다. 기준일은 2026-09-12, 로컬 검토 기준은 `aa317c7`, 기존 실행 검증 환경은 Windows / Node `24.19.0` / Claude `2.1.269` / Codex `0.154.0`이다. 웹의 현재 문서가 이 바이너리 조합에서 그대로 동작한다는 뜻은 아니다.

## 1. 결론과 판정 범위

### 필요한 다섯 가지 작업

1. **완료를 기계적으로 판정할 작업 상태와 자동 재개 절차를 만든다.** Claude의 최종 답변이나 exit 0만으로 다음 작업으로 넘어가지 않는다. 작업 ID, 세션 ID, 마지막 검증 결과, 실제 도구 효과와 다음 단계를 함께 확인한다.
2. **장시간 실행을 끊는 인증·사용량 제한·메모리 대기를 처리한다.** 현재의 짧은 재시도와 인증 파일 재조회만으로는 이 조건을 충족하지 못한다. 정상 인증 갱신의 담당 주체와 계정 일치 조건을 확정한다.
3. **기본 400K/320K 구성에서 여러 차례 압축한 뒤에도 개발 상태가 유지되는지 확인한다.** 기억 문장만 회상하는 시험과 실제 미완료 기능·테스트·도구 이력을 이어가는 시험을 구분한다.
4. **실패 시점을 통제하는 시험을 추가한다.** 응답 전·후, 도구 효과 전·후, 기록 전·후, 부모·자식 종료, 저장 공간·메모리 부족을 각각 재현한다. 실패를 숨기거나 실패한 도구를 무조건 다시 실행해서 통과시키지 않는다.
5. **동일 후보 빌드로 4시간 → 24시간 3회 → 72시간의 단계 시험을 수행한다.** 시간만 채우지 말고 실제 개발 단계·압축·인증 갱신·자식 작업·복구 사건 수를 함께 충족한다. 이것은 제안한 시험 계획이지 업계 인증 기준이나 개발 소요 시간 추정이 아니다.

### 최종적으로 구분할 세 가지 주장

| 주장 | 필요한 증거 | 현재 판단 |
|---|---|---|
| Clauduct에 세션 누적 요청 수·시간의 고정 종료 제한이 없다 | 구현과 누적 요청 회귀 | 관련 구현·합성 검사가 존재 |
| 특정 구성에서 24/72시간 무인 개발을 완주한다 | 같은 구성의 실제 개발·장애·복구·최종 결과 증거 | 신규 검증 필요 |
| 모든 기능·모든 버전에서 영원히 오류나 사람 개입이 없다 | 유한한 시험으로는 성립시킬 수 없는 주장 | 출하 문구로 사용 불가 |

외부 서비스 장애를 제품 지표에서 지워서는 안 된다. 전체 작업 완료율과 Clauduct 자체 결함률을 함께 보고하고, 외부 장애 중의 대기와 복구 시간도 전체 경과 시간에 포함한다. Google SRE가 설명하는 가용성·정확성·내구성 지표의 구분을 이 제품의 작업 단위에 적용한 제안이다. [1: Google SRE](https://sre.google/workbook/implementing-slos/)

## 2. 현재 증거와 새로 확인한 공백

아래의 ‘실행 기록’은 보존된 감사·릴리즈 문서에 근거한다. ‘코드 확인’은 이번 조사에서 현재 파일을 읽어 확인한 사항이며 신규 실사용 재현과 다르다.

| 항목 | 현재 증거 | 무인 연속 개발에서 남는 문제 |
|---|---|---|
| 기본 기능 | 릴리즈 기록의 회귀 23/23, PoC 6/6, HTTP 88/88. 실제 Read/Edit·Bash·PowerShell·MCP·검색·PNG·Agent·Workflow 성공 | 파일 단위 통과에 내부 `notRun`이 포함될 수 있음. 모든 기능 조합의 성공률은 아님 |
| 실제 연속 개발 | 과거 run-04에서 2시간 37분, 408요청, 자식 9명, 개발 3주기, 개입 0 | 성공 398·실패 10. 자연 발생 장애와 의도적 취소를 구분해야 함. 당시 빌드는 최종 릴리즈와 동일하지 않음 |
| 결과 보존 | MCP 효과 1회 후 오류를 주입하고 명시적 resume으로 복구 | 자동 재개기가 장애를 판단하고 재시작한 시험은 아님. 도구 효과 직후 기록 전 crash와도 다름 |
| 기본 압축 | 400K 창·320K 목표 설정. 축소 창에서 메인·자식 발동 관측 | 기본값 발동 및 반복 압축 후 실제 개발 상태 보존 미검증 |
| 인증 | supplier가 매번 기존 인증 파일을 읽고 계정을 고정 | 직접 토큰 갱신하지 않음. `force=true`도 파일 재조회이며 refresh endpoint 호출이 아님 |
| 재시도 | transport 기본 최대 재시도 5회, base 100ms, exponential cap 2초, `Retry-After` cap 5초 | 60초 이상 제한을 더 일찍 재시도할 수 있음. 현재 지연 계산에는 jitter가 없음. native 재시도는 0으로 설정 |
| 메모리·큐 | 메모리에 기반한 admission과 큐 상한 128개 | admission 자체의 대기 기한은 없음. 과거 최대 443239ms 대기 후 client 이탈 사례가 있음 |
| 진단 | 최근 16요청, 실패 처음 8·마지막 8개, 생략 건수. 종료 시 JSONL append | 장기 장애의 모든 전이를 재구성할 수 없음. 강제 종료 전 기록 보존을 입증하지 못함. 로그 회전도 없음 |
| 프로세스 | 정상 종료 cleanup 9개, background worker의 TaskStop 종료 성공 | wrapper/native crash, 손자 프로세스, OS 재시작, 부모 사망 시 생존 worker의 전체 실측 없음 |
| 상태 크기 | Workflow journal 128KiB, 자식 transcript 1MiB 등 제한된 읽기. 일반 agent transcript는 tail 읽기 | 긴 기록의 경계에서 증거를 못 찾거나 크기 거부가 발생하는지 시험 필요. 보안용 제한을 무작정 늘리면 안 됨 |
| 검증기 | native 검증기 제한 90초 기본·120초 최대. stdout/stderr를 `ReadToEndAsync()`로 모음 | 그대로 수일 실행하면 검증기 자체 메모리와 관측 누락이 결과를 오염시킬 수 있음 |
| 부하 회귀 | `test-native.mjs`에 1000요청 검사와 선택적 `--soak-seconds` 경로가 이미 있음 | 합성 gateway 시험이다. 실제 Claude·인증·압축·OS 프로세스 생존의 증거로 승격할 수 없음 |
| 모델·버전 | 네 모델 low 실호출 성공. Astra upstream 오류 1건 원인 미확정 | 실제 기본 역할인 Plan sol/xhigh·일반 자식 luna/max의 장시간 부하를 low smoke로 대체할 수 없음 |
| 미지원 기능 | 다중·실패·취소 알림 자동 복귀, 일부 Workflow 재개·중첩 등이 미지원 또는 미보장 | 이런 기능을 사용하는 개발을 무인 지원이라고 선언하려면 구현·검증이 추가로 필요 |

로컬 근거: [현행 검증표](remaining-verification.md), [릴리즈 실행 기록](release-readiness.md), [run-04 감사](audit-2026-09-11-run-04-exit-diagnostics.md), [transport](../src/native-transport.mjs), [credential supplier](../poc/user-session.mjs), [admission](../src/request-admission.mjs), [상태 projection](../src/request-status.mjs), [종료 기록](../src/clauduct.mjs), [실제 native 검증기](../verification/verify-native-headless.ps1), [합성 누적 검사](../src/test-native.mjs), [Workflow 선택](../src/workflow-selection.mjs).

과거 문서에는 실제 인증 갱신·수시간 실행, 버전 pin·app-server 전환 등이 ‘범위밖’으로 기록돼 있다. 장시간 무인 사용을 새 출하 목표로 삼으면 **인증 유지와 장시간 실행은 필수 검증 범위로 재분류**해야 한다. app-server 전환까지 필수라는 뜻은 아니다. 이전 범위 제외를 남겨 둔 채 전체 완료로 표시하면 목표와 증거가 어긋난다.

## 3. 외부 자료가 제시하는 설계 원칙

### 3.1 자동 압축만으로 장기 개발이 완성되지는 않는다

Anthropic의 2025-11-26 연구는 여러 컨텍스트 창에서 작업이 이어질 때 미완료 기능과 조기 완료 선언 문제가 남는다고 설명한다. 기능 목록, 진행 기록, Git 상태, 실행 가능한 검증을 다음 작업에 연결하는 방식이 도움이 됐다. 이 결과를 Clauduct에 적용하면, 요약된 대화와 별개로 **무엇을 완료했고 무엇이 남았는지를 검증 가능한 로컬 산출물에 연결하는 것**이 필요하다. [2: Anthropic](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)

2026-03-24 후속 연구는 생성 결과를 구체적인 작업 계약과 실행 검사로 평가하는 구조를 강조한다. 여러 에이전트를 무조건 추가하라는 근거는 아니다. Clauduct에서는 기존 회귀·외부 판정기를 먼저 활용하고, 모델 평가자는 자동 검사가 판정할 수 없는 영역에만 보조적으로 두는 것이 변경 범위가 작다. [3: Anthropic](https://www.anthropic.com/engineering/harness-design-long-running-apps)

### 3.2 한 번 성공과 반복 성공은 다른 지표다

Anthropic의 평가 안내는 여러 시도 중 한 번 성공하는 `pass@k`와 모든 반복이 성공하는 `pass^k`를 구분한다. 장기간 사람이 보지 않는 작업에는 후자의 일관성이 중요하다. 실패한 첫 시도를 버리고 마지막 성공만 저장하면 필요한 신뢰도를 측정할 수 없다. [4: Anthropic](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)

### 3.3 대화 이력은 도구 효과의 트랜잭션 기록이 아니다

AWS의 재시도 설계는 요청 응답을 받지 못했을 때 실제 효과가 발생했는지 불명확할 수 있음을 설명한다. 안전한 반복에는 동일한 의도를 식별하는 키와 실제 효과 측의 원자성·중복 방지가 필요하다. Clauduct가 JSONL에 호출 ID 하나를 적었다고 임의 Bash·MCP 효과가 exactly-once가 되는 것은 아니다. [5: AWS](https://aws.amazon.com/builders-library/making-retries-safe-with-idempotent-APIs/)

Claude Code checkpoint도 Bash에 의한 파일 변경과 여러 종류의 자식 변경을 모두 되돌리지 않는다. 따라서 `--resume` 또는 rewind만으로 전체 작업 복구를 보증할 수 없다. 복구는 현재 파일·실행 결과·외부 효과의 관측을 함께 확인해야 한다. [6: Claude Code checkpointing](https://code.claude.com/docs/en/checkpointing)

### 3.4 API 문서와 현재 gateway의 동작은 별도다

OpenAI의 공개 Responses API는 서버 측 및 독립 endpoint compaction을 문서화한다. 현재 Clauduct는 Claude의 요약 요청을 변환해 Codex backend로 전달하는 구조이며, 공개 `/responses/compact`를 곧바로 사용하는 것과 동일하지 않다. 공개 API 예시의 `context_management` 필드를 현재 backend에 추가하는 변경을 바로 권고하지 않는다. [7: OpenAI Compaction](https://developers.openai.com/api/docs/guides/compaction)

Claude 공식 문서도 gateway/custom model ID에 따라 컨텍스트 창 산정과 환경 변수 적용이 다름을 설명한다. 현재의 400K/320K와 20K 예약량 계산은 해당 구성에서 실제 압축 이벤트·입력 사용량·후속 성공으로 검증해야 한다. 압축 기능이나 창 제한을 끄는 방식으로 통과시키지 않는다. [8: Claude model configuration](https://code.claude.com/docs/en/model-config#correct-the-window-for-a-gateway-or-custom-model-id)

## 4. 최소 구현으로 닫아야 할 다섯 가지 묶음

### 4.1 작업 상태와 자동 재개

기존 `clauduct.cmd -p`와 session resume을 유지하고, 먼저 작은 외부 실행 관리기를 검증용으로 둔다. gateway를 전면 교체하거나 분산 큐·새 프레임워크부터 도입하지 않는다. 관리기가 실제 효용을 입증한 뒤 제품 진입점에 포함할지 결정한다.

관리할 최소 상태는 `taskId`, `sessionId`, `attempt`, 현재 단계, 마지막 확인된 산출물, 판정기 버전과 결과, 진행 중 도구·자식의 참조, `nextEligibleAt`, 남은 예산이다. 프롬프트·토큰·전체 원문 stdout은 상태 파일에 복제하지 않는다. 사용자 작업의 내용은 원래 프로젝트에 두고, 실행 관리기는 검토한 식별자·상태 필드만 소비한다.

권장 상태 전이는 다음과 같다. 이는 제안이며 현재 구현된 API 이름이 아니다.

```text
READY → RUNNING → VERIFYING → VERIFIED → 다음 작업
           │           └→ FIX_NEEDED → 같은 작업의 보완
           ├→ RETRY_WAIT → 상태 확인 → RUNNING
           ├→ RECOVERING → 도구 효과 대조 → 동일 session 재개
           └→ BLOCKED_AUTH / BLOCKED_POLICY / UNKNOWN_EFFECT
```

`result` 메시지와 exit code를 읽은 뒤 파일·테스트·도구 효과를 별도 검사한다. 단순 heartbeat, 토큰 발생, assistant의 “계속 진행 중”은 개발 진척이 아니다. 같은 진단·같은 파일 해시·같은 실패가 세 번 반복되면 재개 횟수를 늘리지 말고 원인 분석 또는 명시적인 blocked 상태로 전환한다. 새로운 정보 없이 동일 작업을 무한 재전송하는 것은 무인 개발 성공이 아니다.

동일 세션에는 실행 소유자를 하나만 둔다. 재시작 전 이전 프로세스의 종료와 자식 상태를 확인하고, 살아 있으면 관측을 이어간다. 종료 여부가 불명확한데 새 프로세스를 띄우지 않는다. native 문서는 같은 세션을 두 터미널에서 동시에 resume하면 transcript가 섞일 수 있음을 명시한다. [9: Claude sessions](https://code.claude.com/docs/en/sessions)

일반 도구마다 별도 거대 트랜잭션 계층을 만들기보다 복구 가능한 작업부터 판정한다. 예를 들어 Read는 다시 읽을 수 있고, 파일 Edit는 기대한 이전·이후 내용을 대조할 수 있으며, 테스트는 그 실행이 외부 쓰기를 하지 않는지 확인한 뒤 반복할 수 있다. DB 쓰기·메일 전송·배포처럼 효과를 확인할 수 없는 작업은 대상 시스템의 idempotency 또는 결과 조회가 필요하다. `UNKNOWN_EFFECT`에서 자동 재실행하지 않는 것은 안전성 합격일 수 있지만, 해당 작업의 무인 완주 합격은 아니다.

### 4.2 인증 유지와 서비스 대기

최우선은 토큰 만료 이후 누가 공식 인증 갱신을 수행하는지 정하는 것이다. 현재 supplier는 캐시를 읽을 뿐이므로 다른 정상 Codex 프로세스가 우연히 갱신해 주는 환경에 의존하면 시험을 재현할 수 없다.

OpenAI 공식 안내는 정상 Codex 실행의 내장 갱신을 설명하고, 같은 인증 파일을 여러 동시 작업·여러 머신에서 공유하지 말라고 안내한다. 갱신 토큰의 만료·철회 또는 경쟁 갱신 이후에는 재로그인이 필요할 수 있으며 영구 세션을 보장하지 않는다. **현재 Clauduct가 이 내장 경로를 실행한다는 뜻은 아니다.** [10: OpenAI account auth maintenance](https://learn.chatgpt.com/docs/auth/ci-cd-auth)

권장 검토 순서는 기존 구조에서 공식 인증 관리 주체를 단일화할 수 있는지 확인하는 것이다. 불가능하거나 유지 비용이 크면 문서화된 app-server 인증·요청 경로를 별도 기술 검증 후보로 비교한다. app-server는 managed auth 및 계정 상태 API를 제공하지만, 외부 관리 토큰 모드의 refresh callback은 토큰을 자체 생성해 주는 기능이 아니다. 이 전환은 현재 bridge의 검색·reasoning·도구 프로토콜·비용 계약을 바꿀 수 있으므로 즉시 교체 대상으로 보지 않는다. [11: OpenAI App Server](https://learn.chatgpt.com/docs/app-server)

`401 → 같은 만료 파일 재조회 → 401` 루프, 동일 계정 정상 갱신, 원자적 파일 교체 중 읽기, 다른 계정으로 변경, 갱신 철회, 재로그인이 필요한 상태를 구분해 시험한다. 계정 변경은 자동 계정 전환으로 해결하지 않는다. 이번 제안은 인증 파일 수정·복사·실계정 전환을 실행하지 않는다.

HTTP `Retry-After`는 다음 요청 전 대기 시간을 뜻하며 초 단위와 HTTP 날짜를 허용한다. 현재 5초 cap은 60초·300초 대기 응답에서 일찍 재시도하는 구조다. 재현 검사를 추가하고, 짧은 transport 재시도 예산을 넘으면 상태를 보존해 **허용된 시각까지 작업 수준에서 대기**하는 방식을 권고한다. 무조건 큰 sleep으로 바꾸거나 해당 헤더를 신뢰해 무한 대기하지 않는다. [12: RFC 9110 §10.2.3](https://www.rfc-editor.org/rfc/rfc9110.html#name-retry-after)

동시 자식의 재시도 집중에는 jitter가 도움이 된다. 다만 서버가 지정한 최소 대기를 깎는 jitter는 적용하지 않는다. `429`, `5xx`, 연결 오류, 이미 내용이 전달된 스트림 오류, quota 소진, invalid prompt를 각각 분류하고 재시도 책임 계층을 하나로 정한다. 현재 native 재시도 0과 비스트리밍 fallback 차단을 단순 해제하면 중복 실행 방지가 깨질 수 있다. [13: AWS backoff and jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)

### 4.3 기본 압축과 기록 경계

기본 설정 그대로 실제 개발 자료가 쌓여 압축되는 시험을 만든다. 난수나 같은 문장을 반복해서 토큰 수를 채우는 시험은 초기 parser 검사에만 쓰고 출하 증거로 삼지 않는다. 변경 요구사항·이미 통과한 테스트·실패 원인·도구의 완료 결과·부모 자식 관계가 누적되는 현실적인 개발 과제를 사용한다.

각 압축 직전과 직후에 필수 사실을 판정기로 비교한다. 필수 사실은 목표, 금지 효과, 남은 요구사항, 완료된 부작용, 미완료 도구, 현재 Git 상태, 실패한 테스트, 다음 작업이다. 문장 전체를 그대로 회상하는 대신 그 정보를 바탕으로 올바른 다음 개발 행동을 하는지 본다. native 문서는 compaction 후 어떤 파일과 지침이 재주입되고 어떤 내용이 요약되는지 구분한다. [14: Claude context window](https://code.claude.com/docs/en/context-window)

메인 반복 압축, 자식 압축, 압축 중 upstream 오류, 압축 직후 process resume, tool_result가 가까이에 있는 경계를 모두 다룬다. 실제 발동량이 320K 목표와 다르면 계측·native 예약량·모델 ID 인식·backend 거부를 먼저 조사한다. 목표값을 관측값으로 바꿔 적는 것만으로 완료 처리하지 않는다.

Workflow journal 128KiB, transcript 1MiB 및 일반 agent tail 영역은 임계값 바로 아래·같음·초과를 합성 파일로 시험한다. 제한 확대보다 필요한 레코드를 제한된 읽기로 찾는 방식이 가능한지 먼저 본다. 무결성·경로·신원 확인을 유지해야 하며 큰 파일을 통째로 메모리에 읽어 제한을 없애는 해법은 피한다.

### 4.4 병렬 작업·Windows 종료·자원

부하는 ‘무조건 최대 병렬’이 아니라 메인 1개에 자식 1 → 2 → 4개 순서로 올린다. 자식 9명이 존재했던 과거 기록을 동시 실행 상한 9의 검증으로 해석하지 않는다. admission 큐 대기, 실제 upstream 동시 요청, 프로세스별 Private Bytes·Working Set·handle 수, OS 가용 메모리, socket·worker 수를 함께 측정한다.

정상 진행 중인 긴 빌드와 응답이 없는 worker를 구분하고, 제한 시간은 개발 단계별로 정한다. low 단문 요청의 120초 기준을 sol/xhigh 또는 luna/max의 긴 추론·Workflow에 그대로 적용하지 않는다. 메모리 부족에서는 새 작업 admission을 늦추되, 큐가 클라이언트 제한 시간보다 오래 묵어 사라지지 않도록 대기 상태와 재예약을 검증한다.

Windows는 Unix SIGTERM 의미를 그대로 가정할 수 없다. 일반 종료, native만 종료, wrapper만 종료, 강제 process tree 종료, 터미널 창 닫기, 절전·복귀를 각각 확인한다. 강제 종료와 OS 재시작 시험은 전용 시험 환경에서 수행한다. Job Object는 관련 프로세스를 한 단위로 종료·계측할 수 있는 Windows 네이티브 선택지다. 기존 종료 수단이 충족하면 재사용하고, 부모 사망 시 누수가 재현될 때 적용을 검토한다. breakaway·손자 process·자식이 별도 생성한 process까지 실측해야 한다. [15: Microsoft Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects)

현재 공식 headless 문서는 background Bash와 background subagent/Workflow의 종료 대기를 다르게 설명하고, `-p`와 `--bg` 충돌 및 대기 상한도 명시한다. 새 문서의 시간 값을 설치 버전에 대한 관측으로 사용하지 말고 호환성 시험 항목으로 삼는다. 세션 분리 `--bg`와 도구 `run_in_background`는 다른 기능이다. [16: Claude programmatic usage](https://code.claude.com/docs/en/headless)

### 4.5 판정기와 관측 데이터

현재 종료 snapshot을 진단용으로 유지하되, 시험에서는 각 요청·재시도·도구·자식의 전이를 누락 없이 대조할 수 있는 별도 bounded 기록이 필요하다. 실행 중에 기록하며 record sequence·task/session 참조·고정 category·시각·카운터만 저장한다. 잘린 마지막 JSONL 행, 중복 행, 기록 실패·디스크 부족을 판정 가능하게 한다. 로그 회전 뒤에도 합계와 파일 연속성을 확인해야 한다.

모든 원문을 로그에 저장하는 방식은 채택하지 않는다. 실제 개인 저장소·대화·인증은 복사하지 않고 공개/합성 fixture를 사용한다. 문제 원인 분류에는 상태 코드, allowlist 오류 라벨, 요청 단계, 전달 여부, 시각을 우선 사용한다. 원문 대신 해시를 남기는 것도 가능한 비밀값의 불필요한 복제 수단이 되어서는 안 된다.

검증기의 stdout/stderr는 bounded streaming 소비로 바꾸는 별도 시험이 필요하다. 읽기 속도를 늦추고 pipe를 닫고 마지막 result를 크게 만드는 사례를 포함한다. 최종 result 1개, 실패 이력, 실제 효과를 맞춰 보고 native result를 받지 못했는데 정상 완료로 세지 않는다.

독립 판정기는 agent가 수정한 테스트의 성공만 믿지 않는다. 합격 기준과 검증 oracle은 작업용 코드와 분리해 보호하고, 시험 시작 시 고정한 판정기의 hash와 실제 산출물을 대조한다. 실패한 시험을 삭제하거나 기준을 낮추는 행동을 거부해야 한다. 사용자 환경의 hooks·permissions는 그대로 적용한 상태에서 필요한 정상 작업이 막히지 않는지도 측정한다. `dontAsk`는 미승인 작업을 허용하는 설정이 아니라 거부하는 모드이며, 권한 장치를 제거하는 것은 무인성 개선의 증거가 아니다. [17: Claude permissions](https://code.claude.com/docs/en/permissions)

## 5. 반드시 포함할 장애·경계 시험

시험 결과에는 `scenarioPassed`와 `taskCompleted`를 분리한다. 예를 들어 만료·철회된 인증을 정확히 거부한 시험은 `scenarioPassed=true`일 수 있지만, 개발 작업을 마치지 못했다면 `taskCompleted=false`다. 의도적으로 주입한 실패는 제품의 자연 오류율과 분리하되 원래 기록에서 삭제하지 않는다.

| ID | 시험 자극 | 필요한 관측과 합격 기준 |
|---|---|---|
| F01 | 응답 전달 전 연결 실패·DNS/TLS 실패·일시 단절 | 분류가 고정 라벨로 남고 재시도 횟수·대기가 예산 안에 있음. TLS 검증을 끄지 않음. 회복 가능한 경우 실제 완료 |
| F02 | `429` + Retry-After 60초·300초·날짜·이상값 | 유효한 최소 대기를 앞당기지 않음. 이상값에도 무한 대기/폭주 없음. quota 회복 뒤 같은 작업 완료 |
| F03 | `503` 연속, 일부 복구, 다시 실패 | 짧은 transport 재시도와 작업 수준 재개가 서로 증폭되지 않음. 긴 장애는 보존된 대기 상태로 표시 |
| F04 | HTTP 200 뒤 `error`, 특히 Astra `OTHER` 형태 | HTTP 성공과 모델 완료를 혼동하지 않음. 원인 불명 항목은 그대로 남기고 동일 payload 무한 재전송 금지 |
| F05 | 첫 text delta 이후 SSE 단절·invalid UTF-8·순서 불일치 | 실패한 스트림의 새 도구를 실행하지 않음. 부분 답변을 완료로 표시하지 않음. 가능한 경로에서 상태 대조 후 재개 |
| F06 | 도구 effect 직전/직후, 결과 기록 직전/직후 crash | 실제 effect count·도구 ID·기록을 대조. 중복 0, 확인된 효과 유실 0. 효과가 불명확하면 재실행 대신 UNKNOWN_EFFECT |
| F07 | 마지막 result 직전 pipe 단절·느린 소비자·대용량 출력 | result·exit·효과의 불일치를 감지. 결과 잘림을 성공으로 세지 않음. 검증기 메모리 상한 유지 |
| F08 | 동일 계정 credential 갱신·만료 중 401·파일 교체 경합 | 공식 갱신 후 수동 조작 없이 다음 요청 성공. 일시적 읽기 경합은 제한된 처리, 부분 토큰 사용 없음 |
| F09 | 철회·다른 계정·갱신 실패 | 계정 변경 거부와 작업 상태 보존. 자동 계정 순환·조용한 다른 과금 경로 전환 없음. 무인 완주에는 실패/예외로 보고 |
| F10 | 기본 창에서 반복 압축 및 압축 중 오류 | 실제 compact boundary, 압축 뒤 크기, 필수 사실·도구 이력·다음 행동 보존. 기본 구성에서 후속 개발 계속 |
| F11 | 같은 session 동시 resume 시도·관리기 중복 시작 | 실행 소유자 1개. 이미 실행 중이면 중복 launch하지 않음. 파일과 대화 이력의 interleave 없음 |
| F12 | 부모 종료 중 자식 완료·늦은 알림·중복 알림 | 자식 결과를 올바른 부모/요청에 연결. 같은 효과 재수행 0. 유효한 미지원 알림 경로는 별도 미완료로 유지 |
| F13 | 자식 A 취소 + B 정상 진행 + A의 늦은 응답 | A만 취소되고 B 결과·모델·effort·파일이 보존됨. 재등록된 A에 과거 결과가 섞이지 않음 |
| F14 | Workflow 캐시 없음에서 resume·오래된 journal·대용량 기록 | 정상 지원 여부를 명시. 기대 성공 경로의 재구성이 실제로 성공해야 전체 Workflow 무인성 승격 |
| F15 | 메모리 압력·큐 포화·저속 upstream | OOM·무한 대기 없이 backpressure. 큐 취소·재예약이 명시됨. 부하 완화 후 정상 요청 처리와 작업 완주 |
| F16 | 디스크 부족·기록 권한 오류·잘린 마지막 JSONL | 기록 유실을 숨기지 않음. 검증 가능한 마지막 상태로 복구. 확인되지 않은 단계의 자동 반복 없음 |
| F17 | native crash·wrapper crash·강제 tree 종료·절전 복귀 | 살아 있는 프로세스를 식별하고 중복 실행 방지. 고아 worker·socket 누수 없음. OS 재시작은 별도 전용 환경 시험 |
| F18 | 중간 재시작·압축·자식 실패가 겹치는 사건 | 개별 시험 통과뿐 아니라 결합 사건에서도 동일한 상태·효과 불변식 유지 |
| F19 | hooks 거부·timeout·오류, permission prompt, MCP 시작 실패 | 필요한 정상 경로는 사전 허용 범위에서 실행. 거부·미응답은 우회하지 않고 명시적으로 blocked. 가짜 PASS 없음 |
| F20 | 악성 지침을 포함한 공개 fixture·도구 결과·가짜 완료 기록 | fixture가 권한·대상·검증 결과를 바꾸지 못함. 검증 oracle과 실행 설정 보호. 원치 않는 전송·파일 변경 0 |
| F21 | 모델·effort·native 버전·알 수 없는 beta/event 변경 | 지정 모델을 조용히 바꾸지 않음. 새 실패는 분류·보존. 버전별 계약 시험 미달 시 자동 승격 금지 |
| F22 | 신원·경로 위조, symlink/junction, 기록 재사용 | 기존 신원·경로·중단 검사를 유지. 동적 링크 시험이 정책상 불가하면 해당 보안 보증은 차단 상태로 유지 |
| F23 | 테스트를 지우거나 결과만 조작하는 작업 agent | 고정된 독립 판정기가 거짓 완료를 검출. 테스트 수 감소·불변식 제거를 개선으로 세지 않음 |

F06은 단순 계산 MCP보다 강한 로컬 시험이 필요하다. 시험 전용 서버가 고정 `operationId`를 받아 원자적으로 증가시키는 카운터와 결과 조회를 제공하게 하고, 효과와 응답 사이에서 중단시킨다. 별도로 idempotency를 제공하지 않는 도구에서는 불확실성을 정확히 감지하는지 시험한다. 합성 서버의 성공을 모든 사용자 MCP의 exactly-once 증거로 확대하지 않는다.

MCP의 `readOnlyHint`·`idempotentHint` 등은 도구가 제공하는 힌트이며 실행 보증이 아니다. 서버가 이를 잘못 선언할 수도 있다. 실제 사용하려는 도구의 효과와 복구 계약을 별도 검증해야 한다. [18: MCP maintainers](https://blog.modelcontextprotocol.io/posts/2026-03-16-tool-annotations/)

## 6. 단계별 실행 계획과 합격 기준

아래 수량과 목표는 **Clauduct에 제안하는 초기 기준**이다. 공급자가 인증한 수치가 아니다. 첫 baseline에서 수행 가능성과 비용을 측정하고 시작 전에 확정한다. 결과가 나쁜 뒤 기준을 낮추어 통과시키지 않는다. 기준을 바꾸면 이유·범위·새 baseline을 남긴다.

### 단계 1 — 기준 고정과 관측기 검증

후보 commit·ZIP hash, Node/Claude/Codex의 버전과 파일 hash, Windows build, RAM·CPU, 모델/effort, 지원할 기능·profile·도구 목록을 실행 manifest로 고정한다. 업데이트를 몰래 막는 전역 설정은 바꾸지 않는다. 실행 도중 바이너리가 바뀌면 같은 표본으로 합치지 않고 세대를 분리한다.

기존 회귀를 그대로 실행하고 suite 내부 skipped/notRun을 별도 수집한다. 기존 native 검증기의 기능 사례와 `test-native.mjs`의 누적 검사·soak 경로를 재사용한다. 일반 러너는 60초 상한이므로 장시간 시험을 우회해 넣지 않는다. 별도 bounded 장시간 실행기가 필요한 이유는 시간뿐 아니라 streaming 수집·OS 관측·재개 판정이 다르기 때문이다.

관측기 자체에는 성공 응답 위장, 중복 result, 누락된 종료 기록, malformed JSONL, 잘린 행, 시계 변화, 기록 실패를 입력한다. **가짜 성공을 한 건이라도 통과시키면 본 실사용 시험 전에 수정**한다.

### 단계 2 — 장애 복구 시험

F01~F23을 적용 가능한 범위에서 실행한다. deterministic 경계 시험은 모든 기대 판정을 만족해야 한다. timing 경합은 바뀐 seed/중단 위치를 고정해 최소 20회 반복하는 것을 초기 제안으로 삼는다. 그 숫자 자체가 신뢰도 인증은 아니다.

먼저 로컬 합성 transport와 실제 로컬 도구 효과로 검사한 뒤, 필요한 사례만 실제 Claude와 backend에 연결한다. failure injection마다 위치와 분류를 기록한다. 한 번의 모델 답변에 fault와 private data·외부 쓰기를 함께 노출하지 않는다. 공개 fixture와 전용 상태를 사용한다.

합격 기준은 효과 중복 0, 확인된 결과 유실 0, 다른 자식/계정/작업 상태 혼입 0, 잘못된 완료 0이다. 복구 가능한 시나리오는 실제 작업 완료까지, 복구 불가능한 시나리오는 정확한 중단·보존까지 확인한다. 둘의 결과 열을 합치지 않는다.

### 단계 3 — 4시간 무인 개발

버그 재현 → 테스트 작성 → 수정 → 재검증 → 다음 요구사항의 실제 작업을 수행한다. 단순 정답 문자열을 계속 요청하는 것은 제외한다. 처음에는 메인 1·자식 최대 2로 시작한다. 작업과 fixture는 충분히 다양하고 실제 입출력 결과를 검증할 수 있어야 한다.

초기 요구 사건은 검증 완료 개발 단계 10개 이상, 정상 resume 1회 이상, 계획된 일시 장애 복구 1회 이상이다. 압축이 자연 발생하면 함께 측정하되, 기본 압축을 못 봤다면 그 항목은 아직 미검증이다. 이 단계 통과는 24시간 시험에 진입할 자격이며 HOLD 전체 해제가 아니다.

### 단계 4 — 24시간 반복 3회

동일 후보 빌드와 판정기를 사용하되 작업 데이터·장애 위치·시작 시각을 바꾼다. 기본 메인 Astra/low, Plan sol/xhigh, 일반 자식 luna/max가 실제 요청으로 관측되게 한다. 사용하려는 대체 메인과 effort도 별도 구성 행으로 검증한다. 하나의 테스트 성공을 다른 구성 전체에 복사하지 않는다.

각 실행은 검증 완료 개발 단계 30개 이상을 초기 목표로 삼는다. 3회 합쳐 기본값 메인 압축 3회 이상, 자식 압축 1회 이상, 실제 동일 계정 정상 갱신 1회 이상, 다른 프로세스에서의 복구, 자식 취소와 형제 계속 실행을 포함한다. 인증 수명이 24시간보다 길어 갱신이 발생하지 않으면 synthetic 결과로 대신하지 않고 추가 관측이 필요하다고 표시한다. 실제 토큰을 인위적으로 편집해 만료시키지 않는다.

첫 실행 실패를 버리고 네 번째 성공부터 ‘3회 연속’이라고 보고하지 않는다. 수정이 있었다면 새 후보의 3회 결과와 이전 실패를 함께 보존한다. 장애가 전체 환경에 공통으로 영향을 줬다면 독립 표본처럼 취급하지 않는다.

### 단계 5 — 72시간 출하 후보 시험과 운영 재검증

가장 중요한 실제 사용 구성으로 72시간을 연속 관측한다. 초기 사건 목표는 검증 완료 개발 단계 100개 이상, 기본값 메인 압축 10회 이상, 자식 압축 2회 이상, 정상 인증 갱신 2회 이상, 작업 소유권을 보존한 process 재개, 반복된 병렬·취소·복구다. 사건이 부족하면 시간이 지났다는 이유만으로 해당 항목을 통과시키지 않는다. 시험 예산에 도달하면 종료하고 부족한 증거를 기록한다.

개별 실행에서 허용하는 예상 일시 장애는 종료 없는 대기 또는 검증된 재개로 복구되어야 한다. 사람의 `continue`, 재로그인, 프로세스 수동 재시작, 파일 수동 수정이 필요하면 개입으로 기록한다. 영구 서비스 장애·인증 철회는 정상 안전 중단의 증거가 될 수 있지만 무개입 완주율에서는 성공으로 계산하지 않는다.

동일 빌드에서 이 단계가 통과하면 해당 manifest에 대해 `72h unattended verified` 같은 관측 범위를 명시할 수 있다. 전체 지원 프로필의 미완료 행이 남아 있으면 전 제품 무인 사용 HOLD는 부분적으로 남는다. 이후 버전 업데이트, 새로운 실제 실패, 주요 profile·MCP·model 경로 변경 시 관련 계약 시험과 필요한 장시간 시험을 재수행한다.

4시간 + 24시간 3회 + 72시간을 직렬 수행하면 **시험 실행 시간만 최소 148시간(6일 4시간)**이다. 구현·실패 분석·재시험·인증 경계 대기 시간은 별도이며 지금 산정할 근거가 없다. 이 기간을 한 번 통과했다고 무기한 무장애가 증명되지는 않는다.

## 7. 출하 지표와 통계적 해석

### 필수 지표

| 지표 | 분자·분모 또는 측정법 | 제안한 판정 |
|---|---|---|
| 결과 유실 | 외부 판정기에 확인된 효과/산출물 중 복구 후 사라진 수 | 모든 critical 사례에서 0 |
| 도구 효과 중복 | 동일 작업 의도의 실제 중복 효과 수 | 0. 재시도 request 수와 별개 |
| 잘못된 완료 | 독립 판정 실패인데 VERIFIED로 넘긴 단계 수 | 0 |
| 무개입 완주율 | 시작 전에 적격으로 정한 실제 작업 중 개입 없이 완료한 작업 수 / 모든 적격 작업 수 | 단계별 전수 보고. 후보 24시간 3회·72시간 지정 과제는 모두 완료 |
| 자동 복구율 | 회복 가능한 장애 중 예산 안에 원래 작업을 완료한 건수 / 해당 장애 전체 | 주입 critical 사례 100%. 자연 장애는 전체 수·미복구 이유 별도 |
| 재시도 규율 | 정책 위반 횟수, Retry-After 이전 재요청, effect 불명 상태 재실행 | 0 |
| 복구 지연 | 장애 해소/재시도 가능 시각부터 다음 검증된 진척까지 | 초기 p95 5분 이내 제안. 긴 빌드·추론은 단계 SLA를 미리 별도 정의 |
| admission 대기 | 큐 진입부터 admission/취소까지, 대기 중 client 이탈 | 정상 부하 p95 30초 이내를 초기 목표로 측정. client 만료까지 무표시 대기 0 |
| 관측 완전성 | 예정된 시작·완료·재시도·재개 전이 대비 연속 기록 수 | 시험 원장에서 미설명 누락 0. 요약의 생략은 원장과 대조 가능 |
| 자원 안정성 | warm-up 이후 5분 단위 Private Bytes/RSS·handle·socket·자식 수 | 선언한 자원 한도 이내. 작업 종료 뒤 고아 process 0, 대기 자원 기준선 회복 |
| 자원 증가 추세 | 동일 부하 구간의 기울기와 회수 후 하한 비교 | 반복 실행마다 증가하면 실패 분석. 일률적인 RSS 0 증가를 요구하지 않음 |
| 압축 보존 | 각 compact 전후 필수 사실·작업·도구 연결에 대한 독립 검사 | 필수 불변식 누락 0 |
| 인증 유지 | 같은 계정 정상 갱신 뒤 요청 성공·사람 개입 수 | 정상 만료 경계에서 개입 0. 철회는 별도 중단 판정 |
| 비용·사용량 | actual 사용량과 추정값, 누적 요청/토큰/시간 상한 도달 | 사전 예산 초과 0. 상한 도달은 전체 목표 완료가 아님 |

이상의 p95·횟수·시간은 제품 특성에 맞춰 확정할 제안값이다. 표본이 작으면 p95 하나보다 최댓값과 개별 타이밍을 우선 보고한다. Claude의 비용 JSON을 ChatGPT 구독의 실제 비용·잔여 quota와 동일시하지 않는다. 측정되지 않는 비용은 ‘미관측’으로 두고 호출 수·시간 등 강제 가능한 예산으로 보완한다.

### 왜 72시간 PASS만으로 충분하지 않은가

독립적인 단계마다 성공 확률이 99.9%라고 가정해도, 1000단계를 모두 성공할 확률은 `0.999^1000 ≈ 36.77%`다. 실제 작업은 독립적이지 않고 자동 복구도 있으므로 이것을 Clauduct의 실제 실패율로 읽으면 안 된다. 긴 작업에서 반복 성공과 복구가 필요한 이유를 보이는 계산이다.

독립·동일 분포의 작업을 n회 수행해 실패가 0이었다면, 성공 확률의 단측 95% 하한은 `0.05^(1/n)`이다. 이 하한이 99.9% 이상이려면 **2995개의 독립 작업 성공**이 필요하다. 같은 단문을 2995번 보낸 것을 독립 개발 작업 2995개라고 세면 안 된다. 이 수치는 통계 모델에서 직접 계산한 예시이며 초기 출하 시험에 2995개 작업을 강제하는 제안은 아니다.

NIST의 일정 고장률 모델에서 T시간 무고장일 때 MTBF의 단측 95% 하한은 `T / -ln(0.05)`다. 따라서 72시간 무고장은 그 가정 아래 **MTBF 하한 약 24.03시간**에 해당하며 ‘수개월 안전’의 증거가 아니다. 같은 모델로 24시간 임무 성공률 99%를 단측 95% 신뢰로 입증하려면 약 **7154시간의 무고장 시험 노출**이 필요하다. 공유 서버 장애·버전 변경·상관된 workload에서는 이 가정이 깨진다. [19: NIST reliability handbook](https://www.itl.nist.gov/div898/handbook/apr/section4/apr451.htm)

실무적으로는 모든 가능한 시간을 시험하려 하기보다, 위험한 전이를 빠짐없이 시험하고 실제 기간을 점진적으로 늘리며 운영 중 재평가한다. 이것이 영구 PASS 대신 **지원 범위가 명시된 출하와 지속적인 검증**을 권고하는 이유다.

## 8. 구현 우선순위와 출하 결정

### 먼저 처리할 변경

| 순서 | 변경 | 재사용할 위치 | 완료 증거 |
|---|---|---|---|
| 1 | 작업 원장·독립 완료 판정·단일 session 소유·안전한 resume | 기존 native 검증기, session ID, request status | 잘못된 완료 거부, crash 후 효과 중복 0, 자동 resume |
| 2 | 공식 인증 유지 방안, 긴 Retry-After 대기, jitter·재시도 책임 | credential supplier·native transport | 정상 갱신 2회, 긴 제한 후 복구, native/fallback 울타리 유지 |
| 3 | 기본 압축·긴 이력·상태 크기 경계 시험 | compact policy, agent/workflow selection | 실제 기본 boundary 반복, 필수 상태 보존, 제한 초과 정상 처리 |
| 4 | admission 대기·부하·Windows process 관리·bounded 수집 | admission, launcher, 기존 합성 soak | 과거 장시간 큐 문제 재현과 해소, 고아 process 0, 자원 안정 |
| 5 | 최종 후보 장시간 검증·구성별 인증표 | release builder·기존 회귀·새 장시간 실행기 | 같은 commit의 24h 반복·72h 결과와 원장 대조 |

새 실행 관리기는 필요성이 확인된 최소 범위만 추가한다. JSONL 상태 파일이면 시작할 수 있으며 데이터베이스·서비스·스케줄러 설치를 전제하지 않는다. 다만 JSONL 자체가 effect와 원자적으로 commit되는 것은 아니므로, critical 동작에서는 실제 효과 측의 idempotency/결과 확인이 여전히 필요하다. 감독 프로세스나 저장 구조를 새로 만든 것만으로 이 조건이 사라지지 않는다.

### 전 제품 HOLD 해제를 막는 잔여 항목

다음은 핵심 개발 경로의 72시간 합격과 별도로 남겨야 한다. 약속된 기능에 해당하면 완료 전까지 전 제품 무인성 판정은 유지한다.

| 항목 | 처리 기준 |
|---|---|
| 다중·실패·취소 알림 자동 복귀 | 새 지원 요구사항으로 포함하면 구현·정상/경합/위조·실제 재개 검증 필요. 기존 미지원 표시를 PASS로 바꾸면 안 됨 |
| Workflow 캐시 미적중 resume·중첩·custom agentType | 설치된 native가 제공하는 범위부터 확인. 제공 불가 기능은 제품 약속 자체를 명시적으로 조정해야 하며 숨겨서 제외하면 안 됨 |
| 사용자 UI 취소·plan·hook/plugin·MCP 구성 | headless PASS를 대화형 UI 전수 PASS로 사용하지 않음. 실제 지원 profile별 필요한 조합 시험 |
| JPEG/GIF/WebP·PDF/첨부·비스트리밍 등 | 현재 지원/미지원 계약을 유지. 지원을 약속했다면 기능 검증이 필요. 장기 안정성만으로 구현 공백을 닫지 못함 |
| 캐시 적중 | 기능·비용 목표에 필요한 경우 실제 측정. 캐시 miss에서도 장시간 예산과 성능을 견디는지 별도 검사 |
| 동적 symlink/junction | 현재 정책 차단을 유지. 별도로 허용된 시험 환경과 효과 권한이 마련될 때 검증. 다른 도구나 셸로 같은 거부를 우회하지 않음 |
| 모든 모델·effort 조합 | 저비용 smoke와 pairwise 조합 검사는 선별용. 실제 사용 구성별 장시간 근거가 있어야 해당 구성 승격 |
| 새 native/backend 버전 | 변경 감지와 계약 재검증. 현재 버전 숫자·공개 문서만으로 미래 호환성을 보장하지 않음 |

최종 승인 자료는 후보 manifest, 요구사항별 PASS/FAIL/NOT_RUN/BLOCKED 표, 자연 장애와 주입 장애 원장, 독립 산출물 검사, resource 곡선의 원자료, 개입 횟수, 재시도·효과 중복·유실 수, 남은 제한으로 구성한다. 실행 agent가 아닌 판정기가 기준을 적용하고, 원래 요구사항 중 미완료 행이 있으면 전체 완료로 표시하지 않는다.

## 9. 범위·비용·실행 전제

이 보고서 작성은 공개 자료 조사와 로컬 읽기·문서 작성 범위다. 유료 backend 시험, 토큰 갱신, 계정 전환, permission/hook 변경, OS 재시작·강제 종료, 장기 서비스 설치는 수행하지 않았다.

실제 실행안에는 대상 작업 루트, 공개/합성 데이터, 계정과 backend 목적지, 실행별 최대 시간·요청·토큰 또는 확인 가능한 비용 한도, 동시성, 종료 조건을 명시해야 한다. 현재 대화의 리서치 요청을 무기한 유료 실행이나 인증 상태 변경의 승인으로 사용하지 않는다. 과거 세션의 전권 승인 문구도 새 실행의 대상·효과를 자동으로 결정하지 않는다.

일반 개발 과정에서 생기는 금지 효과를 자동 승인해 무인성을 높이지 않는다. 승인된 로컬 작업은 자동으로 수행하고, 외부 게시·공유 DB 쓰기·사용자 데이터 전송 같은 별도 효과는 검토된 범위 안에서만 실행한다. private data·외부 내용·외부 전송 능력이 한 에이전트에 결합되지 않도록 시험 환경을 구성한다. 안전 중단은 명시적으로 보고하고 개발 완주와 별도 평가한다.

## 10. 확인 범위와 출처

**Verified:** 기준 commit의 관련 코드·검증기·현행 문서·과거 run-04 감사를 정적으로 확인했다. 아래 공식 문서와 원저자 연구 본문을 열어 대조했다. 확률·표본 수·시간 계산은 로컬 Python으로 재계산했다.

**Not verified:** 신규 회귀 실행, 실계정 갱신, 실제 320K 압축, 새 장애 주입, 4/24/72시간 시험, 현재 사용자의 전체 profile·UI·MCP 조합. 이 보고서에 제안한 합격값과 실행기는 아직 제품 구현·실행 증거가 아니다.

**Blocked by:** 기존 동적 symlink/junction 시험의 정책 차단이 현행 검증표에 남아 있다. 이번 조사에서는 그 효과를 재시도하지 않았다. 공개 문서에는 현재 private Codex backend와 Clauduct 변환 전체의 호환성 보증이 없으므로 그 부분은 실제 계약 시험이 필요하다.

### 공개 1차 출처

날짜가 명시된 글에는 발행일을 기록했다. 지속 갱신 문서는 고정 발행일 대신 열람 기준일 2026-09-12를 적용한다. 문서의 현재 동작과 로컬 바이너리의 관측을 구분했다. 검색 결과 요약이나 커뮤니티 이슈만으로 핵심 결론을 확정하지 않았다.

1. Steven Thurgood·David Ferguson 외, Google SRE Workbook, [Implementing SLOs](https://sre.google/workbook/implementing-slos/). 작업 완료·정확성·내구성 지표 설계.
2. Justin Young, Anthropic, [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents), 2025-11-26. 여러 컨텍스트에 걸친 진행 기록·기능 목록·검증.
3. Prithvi Rajasekaran, Anthropic, [Harness design for long-running application development](https://www.anthropic.com/engineering/harness-design-long-running-apps), 2026-03-24. 작업 계약과 실행 평가. 해당 연구 구성을 Clauduct에 그대로 이식하지는 않음.
4. Anthropic Engineering, [Demystifying evals for AI agents](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents), 2026-01-09. 반복 성공, 결과와 실행 경로 평가.
5. Malcolm Featonby, Amazon Builders’ Library, [Making retries safe with idempotent APIs](https://aws.amazon.com/builders-library/making-retries-safe-with-idempotent-APIs/). 응답 소실·실제 효과·idempotency 계약.
6. Anthropic, [Checkpointing](https://code.claude.com/docs/en/checkpointing). Bash·일부 자식 변경과 복구 한계.
7. OpenAI, [Compaction](https://developers.openai.com/api/docs/guides/compaction). 공개 Responses API의 서버 측/독립 compaction. private backend 적용 보증이 아님.
8. Anthropic, [Model configuration](https://code.claude.com/docs/en/model-config#correct-the-window-for-a-gateway-or-custom-model-id). gateway model ID별 창 산정과 override 의미.
9. Anthropic, [Manage sessions](https://code.claude.com/docs/en/sessions). resume·동시 session·이력 저장.
10. OpenAI, [Maintain Codex account auth in CI/CD (advanced)](https://learn.chatgpt.com/docs/auth/ci-cd-auth). 내장 갱신·동시 파일 공유 위험·영구 세션 한계. CI 예시의 인증 복사 방법은 이 제안에 적용하지 않음.
11. OpenAI, [Codex App Server](https://learn.chatgpt.com/docs/app-server). managed/external auth와 계정 API. 선택적 구조 검토 근거.
12. IETF, RFC 9110, [HTTP Semantics §10.2.3 Retry-After](https://www.rfc-editor.org/rfc/rfc9110.html#name-retry-after), 2022-06. 다음 요청 전 대기 의미.
13. Marc Brooker, AWS Architecture Blog, [Exponential Backoff And Jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/), 2015-03-04, 2023-05 갱신 설명. 재시도 집중 분산.
14. Anthropic, [Explore the context window](https://code.claude.com/docs/en/context-window). 압축 뒤 재주입·요약·파일 재읽기 차이.
15. Microsoft Learn, [Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects). Windows process group 종료·계측 및 예외.
16. Anthropic, [Run Claude Code programmatically](https://code.claude.com/docs/en/headless). 결과·stream-json·background 종료·headless 동작. 현재 문서와 `2.1.269`의 호환성은 실측 대상.
17. Anthropic, [Configure permissions](https://code.claude.com/docs/en/permissions). 사전 허용·거부·비대화형 권한 의미.
18. Ola Hungerford·Sam Morrow·Luca Chang, MCP maintainers, [Tool Annotations as Risk Vocabulary: What Hints Can and Can't Do](https://blog.modelcontextprotocol.io/posts/2026-03-16-tool-annotations/), 2026-03-16. idempotency 힌트와 실행 보증의 차이.
19. NIST/SEMATECH e-Handbook, [Constant repair rate (HPP/exponential) model](https://www.itl.nist.gov/div898/handbook/apr/section4/apr451.htm). 무고장 노출 시간과 단측 MTBF 신뢰 하한. 독립 Bernoulli 작업 수 예시는 본 보고서의 별도 직접 계산.

### 로컬 증거 목록

| 파일 | 확인한 내용 |
|---|---|
| [remaining-verification.md](remaining-verification.md) | HOLD·기본 context·과거 제외 범위·현행 조건부 항목 |
| [release-readiness.md](release-readiness.md) | 실제 기능별 검증, failure-resume, Astra 미확정 오류, 최종 ZIP |
| [RELEASE.md](../RELEASE.md) | 오류 계약·설치 환경·지원 제한 |
| [run-04 종료 진단 감사](audit-2026-09-11-run-04-exit-diagnostics.md) | 408요청·실패 10건·최대 약 7.4분 admission 대기 |
| [native-transport.mjs](../src/native-transport.mjs) | 재시도·Retry-After cap·401 이후 supplier 재조회 |
| [user-session.mjs](../poc/user-session.mjs) | 캐시 읽기 전용 supplier·계정 일치 |
| [request-admission.mjs](../src/request-admission.mjs) | 메모리 reserve·큐 상한·대기 로직 |
| [request-status.mjs](../src/request-status.mjs) | 최근 16개·실패 first-8/last-8·고정 진단 projection |
| [clauduct.mjs](../src/clauduct.mjs) | native retry/fallback 설정·종료 append·현재 실행 진입점 |
| [compact-policy.mjs](../src/compact-policy.mjs) 및 [models.mjs](../src/models.mjs) | native 요약 template 의존과 context/effort 계약 |
| [agent-selection.mjs](../src/agent-selection.mjs) 및 [workflow-selection.mjs](../src/workflow-selection.mjs) | 메모리 상태·기록 크기·신원·이력 확인 |
| [verify-native-headless.ps1](../verification/verify-native-headless.ps1) | 120초 최대·전체 출력 수집·기능별 판정 |
| [test-native.mjs](../src/test-native.mjs) 및 [run-node-tests.ps1](../src/run-node-tests.ps1) | 합성 1000요청·선택적 soak·일반 러너 60초 상한 |
