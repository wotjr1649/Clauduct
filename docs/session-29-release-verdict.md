# Session-29 로컬 출하 근거

**후속 사용자 요청의 현재 판단:** 추가 개발 검증 비용을 줄이고 출력/설정 계약과 잔여 네 항목을 재평가했다. [로컬 실사용 출하 가능](local-use-release-decision.md)으로 판정한다. 아래 전체 무인 안정성 HOLD와 실패 원자료는 이력으로 보존한다. 설정/모델 사양은 정적 확인했고, 잔여 운영 관찰을 미실행 상태 그대로 명시했다. 기존 목표 complete는 선언하지 않는다.

시작: 2026-09-14T04:14:56Z. 마감: 2026-09-14T07:14:56Z. 초기 후보 `f425cf0ead479a7914ce9b4381bbcff854fae13f`, branch `work/unattended-release-2026-09-13`. 현재 판정 **HOLD**. 구현 worktree의 tracked 상태는 clean이었다. 사용자 루트 README +2/-0와 기존 untracked 자료는 보존한다. 되돌림은 이번 작업의 개별 커밋/이해한 diff 단위로만 가능하며 사용자 branch에 통합하지 않는다.

**현재 사용자 변경:** Session-29 진행 중 디스크 부족 복구를 검증에서 제외했다. F16의 그 부분은 출하 차단에서 제외하며 기록 잘림·기록 오류·미관측 예약 보존과 기존 실패 이력은 유지한다. 시작 시각·deadline·누적 예산은 변경하지 않는다.

**현재 설정 범위:** 사용자는 이 머신의 native Claude 전역 설정에 있는 bypass 모드만 사용한다고 지정했다. `C:\Users\js\.claude\settings.json`을 비밀 노출 없이 읽어 `permissions.defaultMode=bypassPermissions`를 확인했다(`setting.json`은 없음). 다른 모드와 모드 전환 조합은 출하 요구에서 제외한다. Clauduct는 native의 기본 설정 소스/`CLAUDE_CONFIG_DIR`을 보존하고 `--settings`에 permissions를 쓰지 않는다. 전역 설정을 변경하지 않는다. 공개 fixture는 확인한 모드 값만 새 격리 설정에 적용하며 개인 hooks/plugins/env 원문을 복제하거나 전체 개인 profile을 실행한 증거라고 표시하지 않는다.

## 예산과 실행 경계

기준 원장 `.tmp/release-completion-20260914/live-agent-final/result.json`을 직접 대조했다. 요청 323~327, 보수적 327; 관측 입력 1124010/출력 47690; 미관측 입력 예약 786432/출력 예약 196608; 예약 포함 입력 1910442/출력 244298. 관측 시간 3019404ms와 미관측 160000ms도 유지한다. 추가 한도 요청64/입력400000/출력30000, 누적 상한391/2310442/274298이다. 담당 Codex 사용량과 구분한다.

기존 fixture의 입력131072/출력32768은 완료 후 검사다. `src/native-protocol.mjs`는 `usage-enforced-completion`을 선언하고 upstream에 출력 토큰 상한을 보내지 않는다. 일반 Responses API의 `max_output_tokens` 문서는 Codex 구독 endpoint의 지원 증거가 아니다. 기존 PoC에는 해당 인수를 backend가 거부하는 계약/실패 이력이 있다. 생성 중 reasoning을 포함하는 출력 상한을 사전 보장하지 못하므로 새 실호출은 보류한다. 완료 후 관측, 짧은 prompt, 연결 중단을 소비량 상한의 증거로 쓰지 않는다. 관련 독립 로컬 작업은 계속한다.

## 시작 시 요구 대조

과거 F 표의 IN_PROGRESS를 최신 범위에 그대로 복사하지 않는다. 아래 PASS는 명시된 검사 범위에 한하며 기능 전체나 다른 후보의 결과를 뜻하지 않는다. 실행 근거의 hash와 변경 영향은 최종 artifact에 추가한다.

| 요구 | 현재 근거 / 적용 범위 | 현재 남은 차단 |
|---|---|---|
| F01 연결·DNS·TLS·단절 | DNS 공개 주입과 오류 분류, 공개 native 출력 복구. TLS 실제 준비 실패 이력 유지 | HTTP/TCP 제외. 핵심 복구 증거 영향 대조 |
| F02 Retry-After | parser/defer/새 프로세스 복구 PASS. 자연 quota 대기 불필요 | 변경 영향 대조 |
| F03 반복503 | 실제 native 공개503 및 두 모델 효과 뒤 서비스 오류/동일 세션 복구 | 변경 영향 대조 |
| F04 HTTP200 오류 | 두 조합 공개 native 오류·미완료 도구 차단·동일 세션 복구 | 변경 영향 대조 |
| F05 전달 후 프로토콜 오류 | 두 조합 공개 native 단절/UTF-8/순서 오류와 원래 보고서 완료 | 변경 영향 대조 |
| F06 crash·효과 확인 | local-native 읽기/쓰기 뒤 중단·동일 세션 재개, 개발 부분 적용 검사 | 필요한 복구 경계별 근거 대조 |
| F07 pipe·bounded 수집 | 실제 모델 개발과 공개 native 출력 단절/재쓰기0 | 변경 영향 대조 |
| F08 인증 | 합성401/cache 교체/만료/예산 경계 PASS | 정상 갱신은 사용자 제외 |
| F09 철회·계정 경계 | 합성401반복/403/계정 변경/cache 오류 거부 | 실제 계정 조작 불필요, 보호 유지 |
| F10 기본 압축 | 설정·변환·라우팅 근거 유지 | 기본400K/320K 실발동은 사용자 제외 |
| F11 소유·중복 재개 | 배타적 소유/공개 프로세스 경쟁·재시작 정산 중 시작0 | Workflow 이전 worker 생존 경계 추가 필요 |
| F12 부모·자식 완료/실패 | 일반 Agent 및 고정 성공 main relay 두 모델 성공 | 일반 결과/실패·중복·오래된 결과·형제·취소 연결 검사 필요 |
| F13 취소·형제·늦은 응답 | 현재 소스의 공개 native4사례, CANCELLED/형제완료/잔여0 | 선택 변경 회귀 |
| F14 Workflow 재개 | 신규 run·긴 journal·출처 검사. 정상 재개 구현 없음 | 동일 저장 세션/새 프로세스/저장 결과 재사용·필요 agent 재실행/파일 효과 증거 필요 |
| F15 메모리·queue·기한 | admission 기한/회복 PASS. native 동시성 ECONNRESET/cleanup 간헐 실패 | HTTP 이력은 제외하되 admission 독립 회귀 필요 |
| F16 기록 실패·잘림 | v2 원장·미관측 예약·변조/부분 적용 거부 | 기존 registry 등록 거부 보존. disk-full 실제 증거 미확인 |
| F17 종료·고아 | 이전 실제 실행 cleanup9/잔여0 | 이번 새 실행마다 소유자/자식 종료 확인 |
| F18 압축·재시작·자식 실패 결합 | 기존 표 NOT_RUN, 개별 압축과 복구는 대체 증거가 아님 | 기본 압축 제외와 별개로 결합 증거 미확인 |
| F19 hooks/permission/MCP | native 허용/거부·효과0/1 및 로컬 계약 | 현재 약속 설정 범위의 회귀 대조; 임의 개인 구성 전면 보증 아님 |
| F20 악성 입력 | 소스/도구 경계·MCP 쓰기 전 거부·독립oracle | 외부 전송 없는 로컬 음성 검사로 유지 |
| F21 model/effort | 두 조합 일반 Agent/신규 Workflow/개발 실제 성공 | 새 재개 경로 model/effort 검증 필요 |
| F22 경로·위조·기록 재사용 | 기존 정적 경로·신원·변조 보호 | 새 재개 음성 사례 필요. 동적 링크는 사용자 제외 |
| F23 oracle/가짜성공 | 부분 적용/검사 삭제·변조/결과 불일치 거부 | 관련 코드 불변 여부 대조 |
| 기능·설정 | 기존 기능표/옵션표 및68파일 선택 회귀 | 전체 적용 필수 항목과 최신 후보의 영향 대조 |
| 릴리즈 | 기존 ZIP은 다른 commit | 커밋 후보 ZIP·manifest/hash·새 경로 실행 필요 |

## 진행

- 목표 도구에 기존 목표가 없어 최신 목표를 등록했다. 전체 PASS 전에는 완료로 표시하지 않는다.
- 초기 task 관련 프로세스 census에서 실행 중 native/node worker는 관측되지 않았다. 검사 명령 자신의 PowerShell만 확인했다.
- 추가 실제 요청/입력/출력: 0/0/0. 이전 미관측 예약은 그대로다.
- F12 제품: TaskOutput 구조화 결과를 원본 호출/현재 child request/metadata/마지막 응답에 결합하고, 메인의 동일 부모 SendMessage 재개에서 원자적으로 소비한다. 29개 새 검사 및 기존 선택/실패/다중알림 검사가 통과했다. 공개 native sol 실패·성공, luna 실패·취소4사례는 같은 부모/세션, 자식 시작2개, 보고서 쓰기1회, cleanup9/잔여0을 독립 대조했다. `.tmp/session-29-release/relay-native-verified.json`. 최초 wrapper의 상태 필드명 오류/검증기 project 디렉터리 가정 실패는 유지하고 원자료를 재검증했다.
- 넓힌 gateway 검사는 22개 뒤20초 watchdog에서 실패했다. 변경 전 f425cf0을 별도 새 디렉터리에 추출한 기준선도 정확히 같은22개 뒤 watchdog 실패다. `.tmp/session-29-release/gateway-baseline.json`과 `relay-related-tests.json`. 이를 새 코드의 회귀 PASS나 HTTP 수정 완료로 표시하지 않는다.
- F14 원자료 대조: 이전 공개 probe의 구조화 반환 scriptPath와 실제 재호출 인수는 문자 단위 동일(356자, backslash14개)했다. native2.1.269가 정확한 반환 경로를 허용 읽기 범위 검사에서 거부했다. 이 대조 시점에는 현재 native2.1.270 정상 재개가 NOT_RUN이었다. 아래 후속 공개 과제의 관측이 현재 상태다. 과거 실패를 현재 버전의 재현 결과로 표시하지 않는다. 경로 파싱 착오나 단순 버전 차이를 정상 접근 조건의 근거로 삼아 거부를 재시도하지 않았다. 당시 정상 재개 증거가 없었으며, 아래 후속 구현·관측과 구분한다.
- F12 변경 커밋: `2ac9963a3ea18fad09686031f7d6da4ccc3b4033`.
- 출력32768 고정값을 낮출 수 있게 headless/development 실행·예약·fixture 사용량·PowerShell 판독의 한도를 연결했다. 기본값과 과거 원장은 유지한다. `-MaxOutputTokens 30000 -RequirePreGenerationLimit`는 현재 전송의 한계를 명시적으로 거부한다. 실제 headless 진입 preflight에서 실행 디렉터리 생성0/native시작0/credential0/실요청0을 확인했고 `.tmp/session-29-release/live-preflight-blocked.json`에 남겼다. 이 옵션을 빼고 같은 실호출을 진행하지 않는다.
- 예산 검사19개, PowerShell 한도 판독3개 및 기존 관련10파일이 통과했다. 낮은 한도(각 단계 입력10000/출력3000)로 두 조합의 공개 native 출력 단절→이전 worker 종료→새 프로세스 동일 세션 개발 마감을 확인했다. 각8개 고정 응답, sol5805ms/luna5497ms, 묶음 쓰기1/파일 쓰기2, 최초 OUTPUT_PIPE_CLOSED 보존, 실제 backend0이다. `lower-budget-recovery-sol.json`/`lower-budget-recovery-luna.json` 및 first/finish 원자료가 근거다. 일반 저장 세션 재개 성공이며 Workflow 재개 성공으로 대체하지 않는다.
- 예산 보완 커밋: `762b5e41562c3b24bba51f32a79a34c32d0ebd06`. 후속 영향 검사10파일 중9개 통과, headless 기존 실행 디렉터리 거부 검사가 실패했다. 새 토큰 preflight가 기존 경로 검사보다 먼저 실행된 것이 원인이다. 기존 경로/충돌 검사를 먼저 유지하고 파일 생성 전 토큰 preflight를 수행하도록 순서를 바로잡았다. 해당24개 검사와 예산19개 재검증 통과. 실패한 최초 묶음은 `candidate-impact-tests.json`, 수정 검사는 `preflight-order-fixed.json`에 보존한다.
- 기존 증거 manifest12개 hash가 일치했다. 과거 실모델 개발 두 실행의 wrapper 결과 hash·실행 결과·events hash를 대조했고 각각 독립81개 성공을 확인했다. 이전 서비스 오류 native 증거의6종류×2조합도 확인했다. 관련 핵심 전송·프로토콜·모델·admission·소스/oracle11파일은 f425cf0과 동일하다. `.tmp/session-29-release/evidence-reuse.json`에 재사용 범위를 제한했다. 최초 대조의 evidenceHash 대상 파일 착오는 수집기 원문을 읽고 바로잡았으며 실제 실행을 반복하지 않았다.

### 예산 인터페이스

`verification/verify-native-headless.ps1`의 `-MaxInputTokens`/`-MaxOutputTokens`와 `verifyNativeDevelopment()`의 `maxObservedInputTokens`/`maxObservedOutputTokens`는 기존 한도 이하의 **완료 사용량 제한**이다. `-RequirePreGenerationLimit` 또는 `requirePreGenerationLimit: true`는 현재 `VERIFICATION_PREGENERATION_LIMIT_UNAVAILABLE`로 실행 전 거부된다. 관측 한도를 설정했다고 생성량이 그 값에서 멈춘다고 주장하지 않는다. 일반 Responses API의 [max_output_tokens 정의](https://developers.openai.com/api/reference/typescript/resources/responses/methods/create)는 공개 API 계약이며 구독 backend가 이를 지원한다는 증거는 아니다.

## 후보 판정 — HOLD

F12의 명시적 수집·메인 중계는 제품에 구현했고 현재 요청·부모·결과 신원/중복·실패·취소를 검증했다. F15의 admission 기한·정상 회복도 현재 후보에서 통과했다. 기존 native 동시성/HTTP timeout은 제외된 HTTP/TCP 이력으로 보존한다. 수정이 원래 gateway timeout을 해결했다는 주장은 하지 않는다.

| 현재 차단/미검증 | 판정과 필요한 근거 |
|---|---|
| F14 정상 Workflow 재개 | **공개 native 관측 범위 PASS**. 현재2.1.270 bypass의 새 과제에서 동일 session/runId/scriptPath를 새 프로세스로 재개했다. 저장 결과5는 재사용하고 실패 단계만 새 agent로 실행해 최종12를 기록했다. checkpoint 쓰기1·report 쓰기1·이전 worker 종료를 대조했다. 아래 신원/기록 보호46개와 독립 검증이 근거다. 실제 backend 추론 및 임의 긴/편집 script의 재개는 별도 범위다. |
| F16 디스크 부족 복구 | **사용자 제외**. 미실행 이력은 유지하며 출하 차단에서 제외했다. 원장 잘림·기록 오류·미관측 예약과 부분 파일 효과 검사는 유지한다. 기존 registry 등록 거부도 유지한다. |
| F18 결합 사건 | **NOT_RUN**. 압축 설정/변환/라우팅과 새 프로세스 복구, 자식 실패를 각각 검사했다. 압축·재시작·자식 실패가 겹친 사건의 효과/기록 보존 증거로 합산하지 않는다. 기본400K/320K 실발동은 별도로 사용자 제외다. |
| 설정·도구 조합 표의 잔여 | **bypass 범위에 한해 대조**. native 소유의 hooks/MCP 정상·거부 및 현재 launcher 계약 증거는 있다. 프롬프트 캐시 실제 적중과 사용 중인 bypass 환경의 UI/hooks/plugin 통합 증거는 아직 충분하지 않다. 다른 permission 모드나 모드 전환을 요구하지 않는다. JPEG/GIF/WebP 각 두 모델의 기존 실제 왕복6개는 재사용한다. 임의 개인 profile 전체로 범위를 확대하지 않는다. |
| 새 모델 실호출 | **BLOCKED**. 사용자가 요구한 사전 출력 상한을 현재 구독 전송에서 보장할 수 없다. 검증된 전송 계약/동작 없이 상한 옵션을 제거하거나 관측 후 판정으로 대체하지 않는다. |

F21/F22의 새 Workflow 재개 모델·effort는 공개 native 두 모델에서 확인했고, 세션·오래된 기록·변조 경계는 로컬46개 검사에서 확인했다. 기존 일반 Agent·신규 Workflow·변조 거부의 통과는 유지한다. 이 목록 때문에 최종 커밋과 ZIP의 파일 검증이 성공해도 전체 PASS나 목표 완료를 선언하지 않는다.

### 전체 요구의 최종 적용 대조

아래 근거 파일명은 구현 worktree의 `.tmp/session-29-release/` 기준이다. PASS는 적힌 동작과 관측 범위에 한한다. 초기 표의 오래된 차단 문구보다 이 표와 위 현재 차단 목록을 우선한다.

| 요구 | 현재 동작과 관측 | 상태 / 근거 |
|---|---|---|
| F01 | 공개 DNS·연결 오류/복구와 오류 분류 유지. HTTP/TCP는 사용자 제외이며 TLS 실제 준비 실패 이력은 보존 | 범위 내 기존 증거 재사용, TLS 실제 검사는 NOT_RUN / `evidence-reuse.json`, 기존 통합 manifest |
| F02 | Retry-After defer·새 프로세스 복구, 재시도 핵심 소스 동일 | PASS / `evidence-reuse.json` |
| F03 | 반복503, 두 조합 공개 native 실패와 원래 과제 복구 | PASS / `evidence-reuse.json` |
| F04 | HTTP200 오류 본문, 미완료 도구 차단·복구 | PASS / `evidence-reuse.json` |
| F05 | 단절·UTF-8·순서 오류의 전달 후 실패 보존 및 재개 | PASS / `evidence-reuse.json` |
| F06 | 부분 효과 대조·검사 intent/미확정 분리, 두 파일 개발의 새 프로세스 마감 | PASS(관측한 경계) / `candidate-impact-tests.json`, `lower-budget-recovery-sol.json`, `lower-budget-recovery-luna.json` |
| F07 | 출력 단절 뒤 동일 session·source hash, 묶음 쓰기1/파일 쓰기2 보존 | PASS / 두 `lower-budget-recovery-*.json`; 기존 실제 개발 두 모델 독립81개도 재사용 |
| F08 | 합성401·cache 선택/만료·예산 거부 유지 | 기존 회귀 재사용. 정상 인증 갱신은 사용자 제외 |
| F09 | 반복401/403·다른 계정 혼입·인증 cache 오류 거부 | 기존 회귀 재사용 / `evidence-reuse.json`의 동일 인증/전송 소스 |
| F10 | 압축 설정·변환·라우팅 회귀 통과 | PASS(정적/합성) / `candidate-impact-tests.json`; 기본400K/320K 실발동 제외 |
| F11 | 실행 예약·소유자 종료·중복 시작 거부 유지 | PASS(일반 복구/로컬 활성 기록 거부). native worker 생존 중 재개는 경로 읽기 검사에서 먼저 거부되어 생존 worker 판정 경계는 BLOCKED / `workflow-active-sol-observed.json` |
| F12 | TaskOutput 원결과를 직접 부모 SendMessage 재개에 결합, 중복·오래된 요청·형제 혼입·취소 거부 | PASS / `relay-first-tests.json`, `relay-selection-final.json`, `relay-native-verified.json` |
| F13 | 취소 후 재개0·형제 완료·늦은 이벤트 차단 | PASS / 새 relay 취소와 이전 공개 native4사례, 선택 회귀 |
| F14 | 동일 저장 세션/새 프로세스/원본 script로 저장 결과 재사용·실패 agent 재실행·원래 파일 작업 마감 | PASS(짧은 공개 native 과제) / `workflow-clean-sol-verified.json`, `workflow-stored-sol-complete.json`, `workflow-stored-luna-finish.json`; 과거 거부·실패 이력 유지 |
| F15 | admission 대기 기한과 회복 통과 | PASS(독립 admission). gateway22개 뒤 watchdog는 기준선도 동일 실패 / `relay-selection-final.json`, `gateway-baseline.json`; HTTP/TCP 제외 |
| F16 | 낮은 완료 사용량 한도·footer/journal 일치·잘림/예약 보존 | PASS(로컬 검사) / `token-budget-tests.json`, `token-budget-related.json`; registry 등록 기존 거부 보존, 디스크 부족 제외 |
| F17 | 실제 native 소유 프로세스 종료·cleanup9개·잔여0 | PASS(실행별) / relay 및 개발 복구 결과. 최종 package 실행도 별도 census |
| F18 | 압축·프로세스 재시작·자식 실패가 겹친 사건 | NOT_RUN. 개별 성공을 결합 성공으로 승격하지 않음; 기본 압축 발동 부분 제외 |
| F19 | 전역 bypass 설정 상속, launcher 옵션/hook 계약, native permission/MCP 정상·거부 | 범위 내 PASS. 다른 모드는 사용자 제외, bypass 환경의 UI/hooks/plugin 통합 증거는 잔여 |
| F20 | 악성 소스·도구·MCP/외부 전송·oracle 경계, 공격 입력의 로컬 거부 | PASS(검토한 fixture 경계) / `token-budget-tests.json`, `candidate-impact-tests.json`; 임의 일반 프로젝트 전체 보증 없음 |
| F21 | sol/low·luna/max 일반 Agent/신규 Workflow/개발 과거 실제 호출, 새 공개 relay 라우팅 | PASS(관측 경로). 새 Workflow 재개도 공개 native sol/low·luna/max로 관측 / 기존 증거 및 `workflow-stored-*-*.json` |
| F22 | 현재 요청/세션/부모·metadata·journal·정적 경로 위조 거부 | PASS(현재 경로), Workflow 재개 신원·저장 결과/실행 기록 변조 거부46개 PASS / 선택·journal·resume 검사; 동적 링크 제외 |
| F23 | 부분 적용·테스트 삭제·가짜 성공·oracle 변조 거부, 두 파일 독립81개 | PASS(검토한 개발 과제) / `candidate-impact-tests.json`, `evidence-reuse.json` |
| 기능 표 잔여 | 이미지6왕복 재사용; 일반 background·TaskStop·MCP/WebSearch/WebFetch 기존 증거 유지 | 프롬프트 캐시 실제 적중·bypass 환경 UI/hooks/plugin 통합 증거 잔여 / `additional-evidence-reuse.json`, 기존 기능·옵션표 |
| 새 실호출 예산 | 기본32768 완료 관측값을 실행별 낮은 값으로 연결. 생성 전 상한 요구는 전송 전에 거부 | BLOCKED / `live-preflight-blocked.json`; 새 요청/입력/출력0, 기존 예약 불변 |
| 로컬 릴리즈 | 커밋 후보의 명시 파일 ZIP·manifest/hash·재현 빌드·새 경로 실행 | 생성 후 `final-package-verified.json`과 `shipping-evidence.json`에서 별도 관측. 전체 HOLD와 구분 |

최종 ZIP은 이 문서를 포함한 tracked clean 커밋에서 생성한다. 동반 manifest는 commit, 파일 목록·크기·SHA256을 담는다. 새 공백 경로 실행과 선택 회귀, 원증거 hash, 현재 요구 상태, 누적 예산 및 프로세스 census는 작업용 `.tmp/session-29-release/`에 보존하며 실행 임시 자료·인증·개인 profile은 ZIP/커밋에 넣지 않는다.

### Bypass 설정 확인 뒤 추가 관측

`verify-native-result-relay.ps1 -UseCurrentGlobalBypass`는 전역 설정의 모드만 제한적으로 읽고 bypass일 때만 공개 fixture를 시작한다. native init과 저장 transcript에서 실제 bypass를 독립 대조한다. 두 모델의 실패·취소4사례가 통과했으며 추가 실제 모델 호출은0이다. `bypass-*-verified.json`에 원결과·파일효과·cleanup9/잔여0을 연결했다. launcher 검사는 permissions 재정의와 설정 소스 대체가 없음을 확인한다.

기존 실제 개발 두 실행에서 관측한 cache-read usage는 각각0이다(`prior-cache-usage.json`). 모드 범위 변경 전 headless plan probe는 native2.1.270이 EnterPlanMode를 노출하지 않은 상태에서1개 공개 요청 후 PROTOCOL_REJECTED로 끝났다. 원파일 불변/잔여0이며 모드 범위 변경 뒤 재시도하지 않는다. 이 실패는 이제 제외된 모드의 이력이다.

### 저장 Workflow 재개 구현과 현재 근거

사용자가 지정한 현재 bypass에서 새 공개 과제를 실행했다. 과거 거부된2.1.269 run/script를 이동·복제·권한 변경해 재시도하지 않았다. 현재2.1.270은 자신의 반환 scriptPath와 resumeFromRunId를 정상 수락했으며 taskId와 재실행 agentId를 새로 만들었다. native PostToolUse는 모델의 path-only 입력에 원본 script를 추가한 정규화 형태를 전달한다. 이 차이 때문에 누락됐던 제품 연결을 수정했다.

제품은 현재 요청의 path/runId·부모 snapshot, 같은 저장 session의 원본 inline 호출과 native 반환 기록, 변경되지 않은 script SHA256, journal의 일관된 시작/결과/실패, 저장 결과의 실제 native 자식 metadata와 성공 transcript를 대조한다. 현재 시작 시각보다 오래된 활성 자식·사용자 취소·다른 session/parent·변조/잘림/기록 소실·중복/동시 연결을 거부한다. 원본 기록은 파일 신원과 SHA256 prefix로 재확인하고 append만 허용한다. 읽기16MiB/65536레코드/1초 등 기존 한도를 유지한다. 저장 결과 없이 기록 전체를 복구하거나 편집 script를 실행하는 일반 복원기는 아니다.

`src/test-workflow-resume.mjs`의46개 검사가 통과했다. 기존 Workflow 선택36개/journal30개, 결과 중계29개/실패23개, launcher 검사도 통과했다. 일반 Agent 선택은 묶음에서 HTTP 취소 수신 경계가 한 번 실패했으나 변경 전 기준선과 현재 단독 재검사는 통과했다. `workflow-final-related-tests.json`, `agent-selection-baseline-late.json`, `agent-selection-current-recheck.json`에 모두 보존하며 HTTP 실패 해결로 표시하지 않는다.

sol/low 원래 공개 과제는 초기 관측기 실패 및 제품 연결 실패를 보존한 뒤 complete 단계에서 마감했고, luna/max도 처음 실패한 과제를 finish 단계에서 마감했다. 각각 같은 checkpoint와 저장 결과를 유지했다. 최종 소스의 깨끗한 first/resume 두 단계 sol 시험은 `workflow-clean-sol-verified.json`에서12개 공개 transport 호출, cached agent 실행1회, retry 실행2회(최초 실패 포함), 최종12, checkpoint 쓰기1/report 쓰기1, 매 단계 cleanup9/잔여0을 확인했다. 추가 backend 요청/인증 읽기는 모두0이다. 두 모델의 최종 배포본 증거는 후속 artifact에 연결한다.

재현 진입점은 `verification/verify-native-workflow-resume.ps1`이다. `-Model sol -Phase first`가 반환한 정확한 RunRoot로 `-Model sol -Phase resume -RunRoot <returned-root>`를 실행한다(luna도 동일). 각 단계 요청12/30초, source hash 불변·이전 worker 종료·최초 효과 독립 대조 후에만 다음 프로세스를 시작한다. `verification/verify-workflow-resume-evidence.mjs`는 native 원자료와 파일 효과를 별도로 판정한다. 이는 고정 응답을 이용한 실제 native 동작이며 실모델의 계획·개발 능력 증거로 바꾸어 세지 않는다. plain-text 저장 결과는 로컬 검사만 있고, 공개 native 과제는 StructuredOutput을 쓴다.

### 살아 있는 Workflow worker의 추가 음성 관측

현재2.1.270 bypass의 별도 공개 sol/low 과제에서 저장 단계가5를 반환하고 다음 agent 요청이 실제로 진행 중인 상태를 만들었다. 같은 프로세스에서 정확한 반환 scriptPath/runId로 중복 재개를 요청했으나 native가 scriptPath 읽기 범위 검사에서 먼저 거부했다. 거부 전후 원래 agent는 계속 활성 상태였고 새 자식은 시작되지 않았다. 이후 원래 taskId를 TaskStop으로 취소하고 TaskOutput의 killed, cleanup9/잔여0을 확인했다. 원본 script·구조화 반환 경로와 실제 재호출 인수의 일치를 독립 대조했다.

`workflow-active-sol-observed.json`은 관측 확인 true/수용 기준 통과 false다. 이 결과를 살아 있는 worker를 정확히 판별한 성공으로 세지 않는다. 해당 경로 거부를 다른 인수·권한·프로세스로 재현하지 않으며 동일 효과의 추가 모델 시험도 보류한다. 이미 검증한 이전 worker 종료 후 새 프로세스의 저장 Workflow 재개 성공과는 다른 조건이다. 첫 fixture의 meta.description 누락으로 시작 전 실패한2개 공개 요청도 보존했고, 보완된 시험은7개 공개 요청/실제 backend0이었다.
