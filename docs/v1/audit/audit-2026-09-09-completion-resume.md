# Native 완료 알림 복귀 수정

기준 HEAD는 a64bff1, 작업 브랜치는 fix/native-completion-resume이다. 실제 모델 실행은 추가하지 않았다. get_goal 결과는 null이며 과거 인계의 blocked 상태와 달라 새 목표를 만들거나 완료 표시하지 않았다.

## 실제 실패와 설치 코드 대조

Claude Code 설치 파일 2.1.266을 실행하지 않고 읽기 전용으로 확인했다. HIGH-03의 지정 세션과 연결된 두 자식에서 본문·reasoning 없이 상태·시간·ID·관계만 추출했다.

| 기록 | 확인한 구조 |
|---|---|
| aa7f2126d47e23050 201–204행 | 02:39:41.267–269Z, assistant end_turn |
| a41ec6ef1ac9aae36 172행 | 02:40:52.570Z, assistant end_turn, 최종 message.id 존재 |
| aa7f2126d47e23050 205행 | 02:40:52.707Z, user, isMeta=true, 최상위 origin.kind=task-notification, completed, 연결 손자의 task-id/tool-use-id |
| aa7f2126d47e23050 207행 | 02:40:54.469Z, isApiErrorMessage=true |
| 두 metadata | 부모 역할 claude, 손자 역할 Explore/model=haiku, parentAgentId 및 최초 toolUseId 연결 일치, stoppedByUser 없음 |

origin은 message.origin이 아니라 JSONL 행의 최상위 필드다. 원래 request 715의 selection/CALL 및 upstream 미시도 증거는 [HIGH-03 감사](audit-2026-09-09-native-high-03.md)에 남아 있다.

설치 코드의 xGe는 종료한 에이전트의 task-notification을 묶어 Oq로 전달한다. Oq → ke → vt는 transcript와 기존 metadata를 읽고 알림을 origin이 있는 user 메시지로 만든다. 기존 toolUseId/model/name을 Nw에 전달한다. Nw는 SubagentStart를 기다린 뒤 koe로 새 메시지 저장을 시작하고 XKo로 metadata를 처리하며, 두 저장의 완료를 기다리지 않고 요청을 진행한다. Start hook에는 agent_id/agent_type 및 공통 session_id/transcript_path가 있고 알림 ID·완료 상태는 없다. Stop hook도 최종 성공 확정 이벤트가 아니므로 완료의 단독 근거로 쓰지 않는다.

## 변경과 신뢰 경계

기존 최초 생성·SendMessage 복귀 검증을 유지하고, 이미 검증된 부모의 native 완료 알림 복귀를 추가했다. 새로운 hook이나 전역 설정 변경은 없다. source는 verified-completion-resume이다.

1. gateway가 정상 최종 프레임을 전달한 end_turn 응답의 ID와 시각만 해당 검증 이력에 보관한다. 후속 요청 시작은 이전 완료 증거를 지운다. 오래된 동시 요청의 뒤늦은 반환은 새 요청의 완료 증거가 될 수 없다.
2. 첫 복귀 요청에서 같은 projects 경계 안의 부모·자식 JSONL 끝부분을 각각 최대 1MiB 읽는다. JSON 원문은 파일·로그·진단에 복제하지 않는다. metadata는 기존 16KiB 제한과 canonical path 검사를 재사용한다.
3. 부모의 마지막 user/assistant 행이 native task-notification이어야 한다. 세션·수신 agent·UUID·isMeta·시각·외부 XML 헤더를 확인하고, 자식의 검증된 관계 및 최초 toolUseId와 연결한다. output-file 내용은 경로로 사용하지 않는다.
4. 자식의 마지막 assistant end_turn ID가 gateway가 실제 전달한 ID와 같아야 한다. 양쪽 metadata의 역할·모델·이름·최초 호출·부모와 stoppedByUser를 다시 확인한다. 검증 후 완료 증거와 알림을 한 번만 소비하고 원래 모델 선택·부모·reviewContext를 보존한다.

이는 기존과 같은 동일 사용자 native 파일 및 인증된 loopback 등록의 신뢰 경계다. 같은 사용자 권한으로 metadata·transcript를 직접 변조하는 공격자에 대한 암호학적 출처 증명은 아니다. prompt 안의 완료 주장, TaskStop 또는 Stop hook만으로 복귀하지 않는다. 별도 전역 보안 장치를 변경하지 않았다.

## 수용 결과

| 조건 | 로컬 결과 |
|---|---|
| 부모 종료 → 자식 종료 → Start → JSONL 저장 → 재요청 | loopback 정상 반환, luna/max, verified-completion-resume |
| role=claude 부모 및 reviewContext | 기존 요청 모델 정책·원래 부모 관계·리뷰 문맥 보존 |
| failed/killed/blocked 알림, API 오류, tool_use 중간 반환 | 완료 복귀 근거로 거부 |
| 부모/자식 stoppedByUser, 후속 요청 취소, 오래된 응답 | 완료 증거 사용 거부 |
| 중복·새 UUID로 재사용·동시 복귀 | 완료 증거 한 번만 소비, 거부 시 upstream 미시도 |
| 다른 세션·수신자·관계·모델·역할·이름·호출 ID | 거부 |
| origin 없는 동일 문자열·peer origin·일반 user·본문 중첩 알림 | 거부 |
| 5분 초과 또는 미래 알림·알림 이후 답변 | 거부 |
| JSONL 저장 지연·검증 도중 취소 | 제한 시간 내 재확인, 취소는 증거 미소비 |
| 잘못된 JSON/UTF-8·1MiB 초과 단일 행 | 거부; 본문 비노출 |
| 큰 파일의 UTF-8 중간부터 읽는 tail | 첫 불완전 행 제외 후 마지막 알림 검증 |
| symlink/junction 경계 실험 | Node ERR_ACCESS_DENIED로 준비 실패, 미검증 |

Verified:

- 수정 전 test-agent-selection.mjs 통과로 기준선을 확인했다.
- test-completion-selection.mjs: 42개 통과. 합성 파일과 loopback만 사용했다.
- test-agent-selection.mjs: 통과. 기존 명시 모델·inherit·Skill·peer 복귀 검사를 포함한다.
- test-native-gateway.mjs: 21개 통과.
- test-native.mjs: 45개 통과, 실패 0. subprocess는 기존 합성 Node 자식뿐이다.
- git diff --check 및 실제 수정 파일/호출자 검토. 기존 미추적 파일 3개는 보존했다.

Blocked by: symlink/junction 준비는 제한된 Node fs 권한에서 거부됐다. 분리 조건을 잘못 적용한 한 번의 재실행에서도 같은 준비가 거부됐으며 권한 확대나 다른 도구를 통한 생성은 하지 않았다. 해당 검사는 --symlink 선택 항목으로 남기고 기본 결과에도 notRun으로 표시한다. 이 거부를 통과 결과로 세지 않는다.

Not verified: 수정 버전의 실제 Claude 완료 복귀, symlink/junction 동적 검사. 실제 인증/live 실행의 기존 거부는 유지한다.

지원 한계: 한 user 행에 결합된 여러 완료 알림, failed/killed/blocked 알림에 의한 자동 복귀, 완료 행이 1MiB tail 밖인 경우, 5분 초과 지연, gateway 재시작·1024개 이력 퇴출 뒤의 복귀는 이 경로로 지원하지 않는다. 자식 결과가 완료 알림 태그를 그대로 인용한 경우도 보수적으로 거부한다. 실제 native 전체 완료나 모든 모델 경로의 통과를 뜻하지 않는다.

## 다음 실제 확인

사용자가 수정된 실행기로 새 세션을 시작해 부모가 먼저 종료하고 백그라운드 손자가 나중에 완료하는 경로를 한 번 실행해야 한다. 기존 HIGH-03 세션을 재개하는 것만으로는 새 gateway의 검증 이력을 만들 수 없다. 전체 high 리뷰를 다시 돌릴 필요는 없다. 완료 직후 request-status를 수집해 verified-completion-resume, 예상 모델/effort, success=true를 확인하고 새 세션 ID를 제공한다. 자식별 자동 압축·400K·보조 이벤트 실발생과 전체 호출 경로 조사는 별도 미완료 항목이다.
