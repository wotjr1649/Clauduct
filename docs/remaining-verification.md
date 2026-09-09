# 남은 실제 검증

## 이관 후 작업 기준 — 2026-09-09 확정

이 문서를 현재 작업·검증 현황의 기준으로 유지한다. 과거 감사 기록은 증거 링크이며, 아래의 과거 실행 프롬프트를 일괄 재실행하지 않는다. 이관 HEAD a64bff1 이후 완료 알림 복귀를 수정했다. 최신 커밋은 git log와 diff로 확인한다. [완료 알림 수정 감사](audit-2026-09-09-completion-resume.md)에 로컬 검사와 한계를 기록했다.

사용자 확정 범위는 현재 설치 버전의 모델 호출 경로 전체 목록화 및 경로별 호환성 판정이다. 실제 모델을 호출하지 않는 로컬 명령·도구·훅은 GPT 전환 대상과 구분한다. 핵심 개발 경로의 정상 반환, 전체 경로 조사, 지원/미지원/미검증 분류를 각각 평가한다. 미지원 항목을 목록에 적은 것만으로 구현 완료로 처리하지 않는다. 인증 갱신과 수시간 실제 실행은 제외한다.

### 작업 순서와 종료 조건

최신 보완: local inline Workflow의 실행 관계와 nested metadata 연결을 구현했다. [보완 기록](audit-2026-09-09-workflow-link.md)의 로컬 검사는 통과했지만 변경 후 실제 native 성공은 미검증이다. 다음 단계는 새 실행기로 읽기 전용 Workflow 자식 1개를 실행하고 자식 Read·GPT 모델/effort·완료 복귀를 함께 확인하는 것이다. 아래 문서 선로드 순서는 이전 단계의 기록이다.

현재 우선 작업: 문서 선로드 → run-01 수정본 검사 → native Workflow 최소 시험 순서다. `--document-first`의 자식 실행 인자 연결·기존 환경/설정 보존·충돌 옵션 거부는 `test-launcher-native.mjs`에서 통과했고 dry-run도 확인했다. 실제 선로드 준수는 아직 미검증이다. 사용자 실행 시 최초 작업 도구 Read, 문서 전 선택적 Skill/Bash/Agent 부재, 범위 밖 쓰기 부재를 확인한다. 이후 run-01의 강화된 17개 테스트와 inherit 검토를 수행한다. 전역 plugin이나 guard는 변경하지 않는다.

Workflow는 별도 native 도구다. 설치 2.1.266의 번들 작성 참조에서 agent()의 model/effort/agentType 옵션을 확인했지만 실제 Workflow 자식 metadata·GPT 라우팅·완료 복귀는 미검증이다. 앞 단계 통과 후 사용자 운영의 읽기 전용 자식 1개 시험으로 증거를 확보하고, 그 뒤에만 다중 자식 관계를 검토한다. 기존 Agent 정의 성공을 Workflow 성공으로 대체하지 않는다. 실제 인증 실행 제한을 우회하지 않는다.

모델 선택의 최신 수용 기준은 [GPT Agent 선택 계약](gpt-agent-selection-contract.md)이다. 직접 GPT 선택과 명시 inherit의 부모 모델·effort 상속은 필수이며, Claude 별칭 대체 검증과 전체 모델 목록 확장은 이번 범위에서 제외한다.

| 순서 | 작업 | 필요한 증거 / 종료 조건 |
|---|---|---|
| 1 | native task-notification(completed) 복귀 | c19c8b14 request 22에서 verified-completion-resume/luna/max/success=true 및 부모 선종료·최종 반환 확인. [실제 성공](audit-2026-09-09-completion-success.md). 단일 general-purpose 경로 통과, 과거 실패 원인·다중 알림·symlink는 별도 미완료 |
| 2 | 이벤트·라우팅 진단 | c94bb0ac에서 기존 누적 카운터 실제 확인. [요청 관계·단계별 누적](audit-2026-09-09-request-correlation.md) 추가 구현·로컬 검증 완료. 새 필드 실제 출력·보조 이벤트 발생은 미검증. 과거 other 이름 복원 불가 |
| 3 | 현재 설치 버전 전체 호출 경로 목록 | 실행 파일 2.1.266 해시 일치, 활성 plugin 2개와 설치 skill 23개, querySource 리터럴 34종을 [호출 경로 조사](native-call-paths-2026-09-09.md)에 목록화. 동적 전체 목록·우회 provider/fallback 전수 추적·경로별 실제 반환은 미완료 |
| 4 | 남은 회귀·보안 검증 | 아래 기능별 표의 미확인 항목과 설정 격리·취소·복귀·병렬·스트리밍·자원 정리 검증. 이미 통과한 항목은 변경 영향이 있을 때만 반복 |
| 5 | 현재 코드 검토 및 최종 수용 | 오래된 fixture가 아닌 실제 변경 코드의 diff와 관련 호출자를 검토. 핵심 경로에서 예상치 못한 API 실패 없이 결과 반환. 미지원·보류·미검증을 남김없이 보고하며 전체 완료와 부분 완료를 구분 |

현재 버전은 실행 대상과 2.1.266 설치 파일의 SHA256 일치로 재확인했다. 활성 plugin·설치 skill 목록은 수집했고 내장 동적 기능 전체 목록은 미완료다. 발견한 경로와 각 근거는 호출 경로 조사표에 연결하며 이름을 추정해서 완성하지 않는다.

### 기능별 추가 점검 범위

| 범위 | 현 상태 | 다음 검사 |
|---|---|---|
| 일반 대화·도구 왕복 | Read 실증과 이후 사용자 1~6 통과 보고 보존. 모든 도구의 완전한 행렬은 아님 | Read/Edit/Write/Bash, tool search, 설치 도구의 모델 선택과 native 권한 실행을 분리해 목록화. 로컬 fixture로 검증하고 외부 쓰기는 자동 실행하지 않음 |
| 모델 선택 | 네 GPT 시험 정의와 2f96f1df의 비기본 effort 상속·기존 고정 역할 실제 성공. 승인된 --gpt-agents 일반용 인터페이스 구현·로컬 검증 | [최신 계약과 증거](gpt-agent-selection-contract.md). 새 일반용 agent의 실제 native 작업·쓰기·위임 확인이 남음. Claude 별칭 대체 검증은 제외. UI 배너를 backend 증거로 사용하지 않음 |
| 내장 review/skill/fork | low와 high 최종 반환 확인. high 자식 완료 알림 복귀 오류 남음 | 모든 review 수준을 low/high 성공만으로 통과 처리하지 않음. 공통 코드 검사는 재사용하고 수준별로 다른 실행 경로만 실제 검사 |
| 압축·컨텍스트 | 아래 실제 증거 표 참조 | 500K 정책 전달과 backend 실제 용량을 구분. 기본 400K와 개별 자식 자동 압축은 미확인. 비용 큰 증거는 합성 결과와 구분해 보류 가능하되 통과 표시 금지 |
| 스트리밍·재시도·취소 | 긴 low 요청 ping 41회 완료, 로컬 retry/취소 검사 | 텍스트 조기 전달, 도구 최종 검증 전 비전달, 실제 내용 전 최대 5회 재시도, 내용 전달 후 명시적 재개, backpressure 및 종료 정리 보존 |
| 설정·native 공존 | 전역 설정 상속과 사용자 keybindings 해결 보고 보존 | 모델 변경의 전역 오염, 추가 hook의 Clauduct 자식 한정, native 플러그인/설정 상속을 검사. 전역 설정·keybindings·statusline·plugin·trust는 변경하지 않음 |
| 메모리·장기 개발 | admission 대기와 기존 활성 작업 유지 구현 | 누적 실행 제한과 개별 요청/registry 보존 한도를 구분. 소규모 반복·취소·정리·메모리 상한 검사로 보완하며 수시간 실제 실행 완료로 대체하지 않음 |
| 보안 경계 | 관계 검증·metadata 경로/크기·loopback 인증·비밀 비노출 검사 존재 | 완료 알림 위조/재사용/세션 혼동/사용자 중단, path 탈출·symlink·잘못된 인코딩, 이벤트 형식·순서·본문 비노출 등 변경 경로별 공격 사례 검증. guard 완화로 테스트 통과 금지 |

실제 모델 테스트는 로컬 원인 재현과 회귀 검사를 먼저 끝내고 변경 경로만 수행한다. 같은 실패를 재시험하려면 관련 수정이나 새 증거가 있어야 한다. 고정 토큰/호출 예산을 새로 설정하지 않았으며, 이것이 무제한 실제 실행 허용을 뜻하지 않는다. 기존 인증·실행 거부를 유지하고 사용자가 실제 CLI를 실행한다.

## 현재 판정 — 원본 기록 재확인

### 완료 이후 SSE 진단 — 2026-09-09

`EVENT_AFTER_COMPLETION`은 이제 `OTHER`와 구분한다. 요청별 `attempts`에 다음 고정 분류를 추가했다. 원문 SSE, 임의 이벤트명, sequence 값 자체는 기록하지 않는다.

| 필드 | 의미 |
|---|---|
| `terminalState` | `open`: 완료 미관찰, `completed`: response.completed 관찰, `done`: 후속 [DONE] 관찰. 전체 요청 성공을 뜻하지 않음 |
| `postCompletionFrame` | 거부된 첫 후속 데이터 프레임의 allowlist 이벤트 종류 또는 `done`/`other`/`invalid-json`/`oversized`. 16 KiB 초과 데이터는 진단 JSON 파싱을 하지 않음 |
| `postCompletionSequence` | `expected`/`unexpected`/`invalid`/`missing`/`unsequenced`. JSON 분류를 하지 않은 경우 null. 수치 대신 파서의 다음 순서와 관계만 표시 |

완료 뒤 프레임 거부, 중복 [DONE] 거부, 자동 재시도 금지는 변경하지 않았다. 정상 단일 [DONE]과 comment는 기존대로 허용한다. 구문 검사나 크기 제한에서 먼저 거부된 프레임은 후속 데이터 분류가 없을 수 있다.

Verified: Node permission 제한 아래 `src/test-native-transport.mjs`, `src/test-native-gateway.mjs`, `src/test-native-protocol.mjs` 통과. 합성 loopback에서 분할 프레임, 중복 완료·sentinel, 보조 이벤트, 순서 관계, 잘못된 JSON, 크기 상한, 원문 비노출, status allowlist, 거부·무재시도·소켓 정리를 확인했다. 외부 요청과 실제 인증 조회는 0이다.

Verified: 후속 실제 세션 `c970306f-170a-47f2-ab95-5a52bcd8ffa7`의 메인 56행·자식 17행·metadata 1개를 확인했다. 메인 JSONL 47행의 상태에서 새 진단 필드가 출력되며 자식 요청 7/8은 terra/high, definition-model, success=true다. 자식은 Read 1회와 TERRA-READ-COMPLETED 반환에 성공했다. 메인은 terra/xhigh였고 요청 9가 upstream/OTHER, terminalState=open, 단일 시도·무재시도로 실패했다. 상태 수집 시 누계는 6건 시작·5건 성공·1건 실패이며 최종 답변 이후 누계가 아니다.

Not verified: 실제 upstream의 후속 프레임 종류. 위 세션에서 EVENT_AFTER_COMPLETION은 재현되지 않았으며 전체 SSE 실패 해결도 아니다. 과거 `OTHER` 요청의 정확한 원인은 소급 복원할 수 없다. TaskOutput의 `No task found` 원인 역시 미확정이며 별도 조사한다.

### OTHER 분류 보완 — 2026-09-09

`src/native-protocol.mjs`의 고정 `FAILURE_DIAGNOSTIC_CATEGORIES`를 gateway와 request-status가 함께 사용한다. 기존 transport/응답 검증 코드인 INVALID_SSE, INVALID_UTF8, SEQUENCE_MISMATCH, TRUNCATED_STREAM, INCOMPLETE_RESPONSE, STREAM_ORDER, SNAPSHOT_MISMATCH, UNSUPPORTED_METADATA_EVENT 등을 이제 개별 분류한다. 원문·임의 오류 코드는 저장하지 않으며 미등록 분류는 gateway에서 OTHER, 상태 입력의 미허용 값은 null로 유지한다. HTTP 응답·프로토콜 검증·재시도 동작은 변경하지 않았다.

Verified: `node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs` 35개 통과. 8종 실제 loopback 오류를 파서부터 상태 조회까지 검증했고, 전체 고정 분류의 전달·미등록 분류 fallback·비노출을 검사했다. 같은 Node 제한으로 test-native-protocol.mjs와 test-native-transport.mjs도 통과했다. 실제 모델 호출·인증 조회·외부 요청은 0이다.

Not verified: 확장한 분류의 실제 native 실패 출력과 요청 9의 정확한 원인. 다음 관찰은 사용자가 새 `--gpt-agents` 실행에서 수행한다. 메인 모델·effort는 사용자의 현재 선택을 유지하고, 오류 발생 직후 기존 request-status.mjs로 분류를 수집한다. 실패한 작업을 자동 반복하거나 이미 성공한 전체 모델·상속·쓰기 행렬을 재실행하지 않는다.

아래 번호별 프롬프트는 과거 검증 절차다. 이미 통과한 항목을 반복 실행하라는 뜻이 아니다. 전체 완료 판정은 보류한다.

| 요구사항 | 원본 증거 | 판정 / 남은 범위 |
|---|---|---|
| compact 구간별 시간 계측 | f236f867 61행 request 11: admission 0.15ms, 첫 이벤트 2212.41ms, 첫 텍스트 10776.14ms, 첫 전달 10776.18ms, 완료 64177.91ms, 재시도 없음 | 실제 계측 확인. upstream 내부의 세부 연산 시간은 미확인 |
| 수동 compact 후 새 Read | 922e6ba0 84행 manual 경계 뒤 112/113행 models.mjs Read 성공 | 통과 |
| compact medium 적용 | f236f867 40행 manual 64806ms, 31511→9820; request 11 requested=max/actual=medium 성공 | 통과. 서로 다른 입력의 max/medium 시간을 통제 실험으로 해석하지 않음 |
| 자동 compact 및 후속 요청 | b6d81841 119행 auto 68608ms, 98097→39330; 142행 request 36 medium 성공; 137/138행 새 Read 성공 | 검증 모드 autoCompactWindow=100000에서 통과. 기본 400K와 각 자식 내부 발동은 미확인 |
| native 역할·환경 상속 | f236f867의 과거 역할·환경 증거 보존. c94bb0ac 원본 request 10/11은 무명시 Plan/role-default/sol/xhigh/success=true | 현재 무명시 Plan 실제 통과. [실제 감사](audit-2026-09-09-plan-lifetime-success.md). 환경 상속은 backend 용량 증거가 아님 |
| gateway 누적 진단 | c94bb0ac 메인 38→57행 started/succeeded 2→7, failed/unsupportedEvents 0 유지 | 실제 출력·정상 증가 통과. 보조 이벤트는 0이므로 실발생 미관찰 |
| 현재 sol/xhigh 명시 선택·재개 | c5a3b05b request 11/12 explicit-metadata, 19/21 verified-resume; 두 Read 및 완료 | 통과. 모델 명시 없는 Plan 역할 검증으로 대체하지 않음 |
| native code-review low 완료 | 268e9bf2 자식 a91eb3a1de1548dde: Bash diff 결과 연결, 최종 지적 반환, 부모 267행 completed; 270행 request 4/5 luna/max native-fork 성공 | low 실제 완료. strict diff 호출 수용과 약 10분 31초 요청의 ping 41회 및 완료 확인. 다른 review 수준·병렬 전체 완료는 미확인 |
| native 병렬 code-review 최종 반환 | HIGH-03: 1d6ef4cd 루트/직접 자식 13개/손자 4개 대상 Read, 부모 518행 completed, 521/522행 ReportFindings 5건. request 717/719 처리 13052.10ms 겹침 | 최종 반환 통과. 자식 task-notification 복귀 CALL 한 건과 도구 오류가 남아 오류 없는 완료는 아님. [상세 감사](audit-2026-09-09-native-high-03.md) |
| peer 메시지 기반 복귀 | HIGH-03 원본 537행 request 717/730/735/740: luna/max verified-peer-resume success=true | 실제 통과. native task-notification 재기동은 별도 경로 |
| 완료 알림 기반 복귀 | c19c8b14 부모 19행 종료 → 자식 23행 종료 → 부모 20행 completed → 부모 23행 최종 반환. 메인 50행 request 22 진단 성공. [실제 성공 감사](audit-2026-09-09-completion-success.md) | 단일 general-purpose 실제 통과. 35985327 실패의 정확한 원인은 미확정 |
| 리뷰 스킬 재호출 감소 | HIGH-03 연결된 18개 실행에서 superpowers/code-review Skill 호출 0, claude-api 3 | 이번 실행에서 확인. 일반적인 모델 준수 보장은 아님 |
| Codex 보조 metadata 이벤트 | HIGH-03 마지막 16개 진단 auxiliaryMetadataEvents=0 | 실제 발생·처리 미확인. 로컬 합성 검사만 통과 |
| 실제 인증 갱신·수시간 실행 | 사용자 지정 제외 | 이번 단계에서 실행하지 않음 |

완료 알림 복귀는 35985327에서 재실패한 뒤 c19c8b14의 단일 general-purpose 경로에서 실제 성공했다. 과거 실패의 정확한 원인은 여전히 미확정이다. 다음 작업은 보조 이벤트 실발생과 미검증 모델 호출 경로의 증거 확인이다. 누적 진단은 새 gateway부터 적용되며 과거 event=other의 원래 이름은 복원할 수 없다. 인증된 실제 실행은 기존 거부를 우회하지 않으며, 합성 검사 통과만으로 미확인 행을 통과 처리하지 않는다.

전제: 기존 live/인증 실행 거부 때문에 에이전트가 실제 Claude를 대신 실행하지 않는다. 사용자가 실행한 세션의 JSONL과 정제된 시간 진단을 확인한다. 전역 설정, hook trust, 모델 기본값, 권한은 변경하지 않는다. 실제 인증 갱신과 수시간 장기 실행은 이번 단계에서 제외한다.

## 1. compact 최적화 적용과 품질

현재 Clauduct를 종료하고 업데이트된 실행기로 기존 세션을 재개한다.

```powershell
D:\AIDEV\Clauduct\clauduct.cmd --model luna --effort max --resume 922e6ba0-e03b-44de-81a2-b68dcb217bf3
```

먼저 아래 짧은 사전 점검 프롬프트를 입력창에 직접 붙여넣는다. 파일을 Read로 읽게 하면 tool_result 경로가 되어 같은 검사가 아니다. 이것은 native /compact 실행이 아니라 실제 Claude 전송 경로에서 템플릿 라우팅만 확인하는 검사다.

```text
CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.

- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.

This is a compact routing preflight, not a conversation summary. Do not analyze the conversation or perform work. Return exactly this fixed response:
<analysis>Routing check only.</analysis><summary>COMPACT-PREFLIGHT-OK</summary>

REMINDER: Do NOT call any tools. Respond with plain text only — an <analysis> block followed by a <summary> block. Tool calls will be rejected and you will fail the task.
```

응답 직후 다음 프롬프트로 결과를 수집한다.

```text
허용된 셸 도구로 node D:/AIDEV/Clauduct/src/request-status.mjs를 정확히 한 번 실행해.
진단 JSON을 생략하거나 고치지 말고 그대로 출력해.
직전 사전 점검 요청에서 purpose=compact-template, requestedEffort=max, effort=medium, compactShape.matches=true, success=true인지 확인해.
뒤따른 일반 진단 요청이 conversation/max인지도 확인해.
기록이 없거나 조건이 다르면 실패 또는 미확인으로 보고하고 중단해.
/compact를 실행하지 마. 파일·설정·권한을 변경하거나 환경 변수·인증 파일을 별도로 조회하지 마.
명령이 거부되거나 실패하면 우회·재시도하지 말고 오류만 보고해.
```

이 사전 점검이 실패하면 긴 compact는 반복하지 않는다. compactShape와 시간 진단부터 분석한다. 성공해도 native의 실제 compact 요청과 같은 형태임을 완전히 입증한 것은 아니므로, 다음 단계에서 manual boundary와 진단 요청 시각을 연결해야 한다.

사전 점검 통과 후에만 다음 명령을 입력한다.

```text
/compact 검증 식별자, 네 모델 기본 effort, 파일 수정 금지와 남은 검증 항목을 보존해.
```

완료 후 아래 프롬프트를 입력한다.

```text
압축 이전 검증 식별자와 파일 수정 금지 조건을 말해.
Read 도구를 새로 호출해 D:/AIDEV/Clauduct/src/models.mjs를 읽고 네 모델의 기본 effort를 표로 답해.
이어서 허용된 셸로 node D:/AIDEV/Clauduct/src/request-status.mjs를 한 번 실행하고 JSON을 그대로 보여줘.
파일·설정을 수정하거나 인증 값을 별도로 조회하지 마. 실패하면 우회하지 말고 보고해.
```

통과: compact 요청에서 purpose=compact-template, requestedEffort=max, effort=medium, 같은 모델과 성공 상태를 확인한다. 이후 일반 요청은 max로 돌아오고 기억 복원·새 Read 결과 연결이 성공해야 한다. duration과 요약 길이를 기록하되 대화 길이가 달라진 순차 실행을 엄밀한 동일 조건 벤치마크라고 부르지 않는다. 템플릿이 일치하지 않으면 목적 분류를 조정하기 전에 설치 native와 요청 형태를 다시 확인한다.

## 2. native 역할과 컨텍스트 환경 상속

같은 세션에서 아래 프롬프트를 입력한다.

```text
Clauduct의 실제 native 서브에이전트 연결을 검증해.
Agent 도구로 Explore, Plan, general-purpose를 각각 한 번씩 순차 실행해. 모델을 명시적으로 지정하지 마.
각 에이전트는 Read로 D:/AIDEV/Clauduct/src/models.mjs를 읽고 자신에게 지정된 역할의 기본 모델과 effort를 코드 근거로 보고해.
각 에이전트가 끝날 때마다 메인의 허용된 셸에서 node D:/AIDEV/Clauduct/src/request-status.mjs를 한 번 실행하고 JSON을 그대로 보여줘.
에이전트에게 셸 도구 실행을 요구하지 마. 역할과 hook 환경은 진단의 role, roleRegistered, model, effort, agentContextPolicy로 확인해.
모델의 자기소개를 실제 실행 모델의 증거로 쓰지 마.
관련 진단 값이 없으면 추측하지 말고 미확인으로 보고해. 도구가 거부되면 권한을 바꾸거나 우회하지 마.
파일이나 설정은 수정하지 마.
```

통과: native Agent 생성 및 역할 hook과 실제 요청 진단에서 Explore/general-purpose=luna/max, Plan=sol/high, roleRegistered=true를 대조한다. 일반 실행에서는 각 agentContextPolicy가 window=500000, autoCompactWindow=500000, compactPercent=83.33333333333334여야 한다. evidence=subagent-start-hook-environment는 실제 hook 프로세스 환경을 뜻하며 native 내부의 최종 context 계산이나 backend 용량까지 증명하지 않는다. 사용자 입력에 단순히 역할명이 있었다는 사실은 증거가 아니다. hook이 생략돼 값이 없으면 미검증으로 유지한다. 기존 hook 등록에 수치가 추가됐으므로 이 버전을 처음 사용하는 경우 gateway를 재시작해야 한다. 최근 16개 요청에서 앞 역할 기록이 밀려나지 않도록 에이전트별로 진단을 수집한다.

## 3. 자동 압축 실제 경로의 저비용 확인

기존 정상 세션과 구분되는 새 실행에서만 검증 옵션을 사용한다.

```powershell
D:\AIDEV\Clauduct\clauduct.cmd --model luna --effort max --verify-auto-compact
```

`/context`로 500K 모델 창을 확인하고, 진단에서 autoCompactWindow=100000을 확인한다. 이 모드는 20K 출력 예약량 조건에서 약 66.7K 자동 발동 목표다. /autocompact 값 지정으로 전역 설정을 바꾸지 않는다. 자동 압축이 기존 설정에서 비활성화돼 있으면 이를 숨기거나 덮어쓰지 않고 해당 조건을 보고한다.

```text
D:/AIDEV/Clauduct의 실제 구현을 읽기 전용으로 검토해.
Read로 src/native-protocol.mjs, src/native-gateway.mjs, src/native-transport.mjs, src/test-native.mjs, src/test-native-protocol.mjs를 순서대로 각각 한 번씩 읽어.
누락된 범위가 있으면 필요한 범위만 추가로 읽되 총 Read 호출은 10회를 넘기지 마.
라우팅, 스트리밍, 오류 처리 검증이 서로 연결되는지 근거를 들어 1000자 이내로 보고해.
/compact를 직접 실행하거나 문맥 채우기용 반복 출력·파일·루프를 만들지 마.
마지막으로 허용된 셸에서 node D:/AIDEV/Clauduct/src/request-status.mjs를 한 번 실행하고 JSON을 보여줘.
파일·설정·권한은 변경하지 마. 도구가 거부되면 우회하지 말고 보고해.
```

통과는 transcript의 compact_boundary에서 trigger=auto, 압축 후 토큰 감소, 후속 요청 성공으로 판정한다. 이 작업량으로 기준에 도달하지 않으면 자동 발동 미관찰로 기록하고 정상 개발 중 관찰을 이어간다. main의 auto compact 성공을 모든 subagent의 auto compact 성공이나 실제 400K/500K backend 수락으로 확대하지 않는다.
