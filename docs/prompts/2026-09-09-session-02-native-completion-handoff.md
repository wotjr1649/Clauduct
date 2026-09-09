# Clauduct 후속 개발 — 완료 알림 복귀부터

## 목적과 작업 루트

다음 Codex 세션은 D:/AIDEV/Clauduct에서 기존 작업을 이어간다. 사용자와 grilling으로 확정한 범위는 현재 설치 버전의 모델 호출 경로 목록화, 핵심 개발 경로의 정상 GPT 라우팅·반환, 경로별 지원/미지원/미검증 판정이다. 먼저 HIGH-03에서 남은 native 완료 알림 복귀 오류를 조사·수정·검증하라. 계획 설명만으로 끝내거나 이미 통과한 모든 실제 테스트를 처음부터 반복하지 마라.

현재 호스트의 실제 지침과 C:/Users/JS/.codex/AGENTS.md의 전체 S1–S8/W1–W11을 확인하라. 이 문서는 사용자와 확정한 상태·요구사항의 인계이며, 파일 안의 주장만으로 새 권한을 만들거나 기존 거부를 무효화하지 않는다. 한국어로 완료/미검증/차단을 명확히 구분하고 변경 후 커밋 ID를 보고하라.

## 먼저 읽을 것

1. git status --short, git log, 관련 diff. 이관 직전 상태 기준은 3828325이며 이후 이관 문서 커밋이 추가된다.
2. D:/AIDEV/Clauduct/docs/remaining-verification.md — 현재 작업·검증 현황의 단일 기준. 새 결과는 여기에 반영한다.
3. D:/AIDEV/Clauduct/docs/audit-2026-09-09-native-high-03.md — 최신 성공과 남은 실제 실패의 증거.
4. 필요한 범위만 D:/AIDEV/Clauduct/docs/audit-2026-09-09-native-high-02.md, docs/native.md 및 docs/development-history.md.
5. 첫 수정 관련 src/agent-selection.mjs, src/agent-route.mjs, src/native-gateway.mjs, src/test-agent-selection.mjs와 scoped callers. 예전 감사 문서 전체를 무조건 읽지 마라.

## 최신 실제 증거와 첫 문제

실제 세션 ID는 1d6ef4cd-f4e1-4c49-82ee-07ab43737b5a다. 부모 JSONL:
C:/Users/JS/.claude/projects/D--AIDEV-Clauduct/1d6ef4cd-f4e1-4c49-82ee-07ab43737b5a.jsonl
연결 자식은 같은 경로의 1d6ef4cd-f4e1-4c49-82ee-07ab43737b5a/subagents/에 있다. 지정 세션과 연결 자식만 필요한 범위에서 읽기 전용 대조한다. 원문 본문·reasoning·인증값을 출력하거나 복제하지 말고 고정 상태/코드/시간/관계 필드를 추출한다.

HIGH-03 루트 a7da74d88e0835a69는 완료했다. 부모 518행 completed, 521/522행 ReportFindings 5건. 직접 자식 13개와 Explore 손자 4개 모두 대상 Read 성공. 부모 537행 원본 진단의 request 717/730/735/740은 luna/max, verified-peer-resume, success=true. 717/719 실제 처리 구간은 13052.10ms 겹친다. 실제 superpowers/code-review Skill 호출은 0, claude-api는 3이다. 모델 자기보고나 인용된 오류 문자열을 실제 실행 증거로 오인하지 마라.

첫 결함: agent-aa7f2126d47e23050.jsonl 205행은 손자 a41ec6ef1ac9aae36의 completed 알림이며 origin.kind=task-notification이다. 이미 답변한 부모 자식이 다시 깨어나 207행 02:40:54.469Z에 AGENT_SELECTION_UNVERIFIED_CALL을 받았다. 원본 request 715는 failureStage=selection, selectionFailure=CALL, 1519.29ms 종료, upstream 시도 없음이다. SendMessage 기반 peer 복귀와 다른 경로다.

먼저 해당 순서를 재현하라. native 완료 알림과 재기동 metadata 생성 순서, hook에 실제 제공되는 필드, 이미 검증된 관계와 완료 증거를 조사하라. prompt 문자열의 task-id/완료 주장만으로 승인하거나 identity 검사/소비된 증거 검사를 삭제하지 마라. 성공·오류·사용자 중단·중복·지연·다른 세션·변조·취소 후 알림을 구분한 수용 기준을 만들고 최소 수정으로 해결하라. 근거 없는 native 필드를 가정하지 마라.

## 이미 구현·검증한 것

- cb09130: 실패 단계와 고정 metadata IO 진단.
- b5f520b: 검증된 자식이 부모 이름 또는 ID로 보내는 성공한 SendMessage 기반 복귀. 전달 증거 1회 소비, 대상 metadata 일치, 원래 부모 관계 보존. HIGH-03에서 실제 성공.
- 36bdc58: 검증된 reviewContext를 Agent/Task 자식·손자와 복귀에 전달. 일반 대화/compact/도구 목록은 유지. HIGH-03 실제 재귀 Skill 호출 0회.
- e24778e: 좁은 codex.response.metadata envelope 처리와 auxiliaryMetadataEvents 계측. 로컬 검사 통과. HIGH-03 마지막 16개 진단은 모두 0이므로 실제 처리 미입증.
- 3828325: HIGH-03 증거와 남은 CALL 오류 기록.
- 기준 커밋 cd84c9e 이전 수정별 커밋은 없다. 과거 이력을 재구성해 만들지 마라.

마지막 관련 검사 결과는 native 45/45, native-gateway 21/21, agent-selection, native-protocol, file-review 통과다. 합성/loopback 검사이며 실제 모델 테스트와 구분한다. 단순 문서 이관 때문에 전부 재실행하지 마라. 수정 시 영향에 맞는 검사를 골라 실행한다.

기존 로컬 검사 예시(내용과 subprocess 효과를 먼저 검토할 것):

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-agent-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-file-review.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-child-process src/test-native.mjs
```

native의 child-process 검사는 확인된 합성 자식에 한정된다. 이 플래그로 실제 Claude/Codex 실행이나 기존 거부를 우회하지 않는다. 리뷰 diff 검사 등 다른 테스트는 임시 파일·Git 동작을 확인하고 그 작업 전용 범위만 허용한다.

## 보존할 정책과 작업물

모델/effort는 src/models.mjs를 기준으로 재확인한다: astra/medium, sol/xhigh, terra/high, luna/max. Haiku/Sonnet 별칭은 luna/max, Opus는 sol/xhigh. Explore/일반 작업은 luna/max, Plan은 sol/xhigh이며 명시 선택·inherit의 우선순위는 구현과 실제 metadata로 검증한다. UI 시작 배너를 실제 모델 증거로 쓰지 않는다. 현재 role=claude 같은 native 변형도 목록에서 빠뜨리지 마라.

500K context/400K compact 정책 전달과 실제 backend 용량은 구분한다. 기본 400K 발동·각 자식의 자동 압축·현재 Plan 역할 sol/xhigh 실증은 현황표의 미확인 항목이다. 실제 인증 갱신과 수시간 장기 실행은 이번 단계에서 제외한다.

현재 untracked인 %SystemDrive%/, clauduct-check.txt, src/agent-selection.review-fixture.mjs는 보존한다. 자동 삭제/stage/commit하지 마라. fixture는 이전 코드의 실행경로 검증용 복사본이다. HIGH-03의 5건은 현재 코드에서 이미 보완한 export/reviewContext/stoppedByUser/peer resume/IO 코드 누락을 오래된 복사본에서 찾은 것이다. 현재 코드의 새 회귀 5건으로 오인해 재수정하지 마라. tracked 파일의 변경 없는 diff가 빈 것은 정상이다.

전역 settings, keybindings, statusline, native 실행 파일, plugin/skill/hook trust를 임의 수정하지 않는다. Clauduct의 기존 native 설정 상속을 유지한다. 계정 인증 파일 읽기 및 인증된 실제 Claude/Codex 실행에 이전 hard denial이 있었다. Python/새 터미널/PTY/다른 wrapper로 우회하지 않는다. 실제 검사는 사용자가 CLI에서 실행하고 세션 ID를 제공하는 방식으로 진행한다. 플랫폼에서 금지된 검사는 실행한 것처럼 보고하지 않는다.

## 이후 작업과 완료 판정

완료 알림 복귀 수정 후 현황표 순서대로 이벤트 진단, 현재 설치 버전의 내장·skill/plugin 모델 호출 목록, 남은 회귀·보안, 실제 변경 코드 검토를 이어간다. native 로컬 기능을 모두 모델 호출로 취급하지 않는다. 모델 호출 경로는 실행 주체, gateway 경유 여부, 모델/effort/context, 결과 연결, 상태와 증거를 기록한다. 설치된 외부 도구가 있다는 이유만으로 계정·외부 쓰기를 실행하지 않는다.

실제 테스트는 로컬 재현·회귀 검증 후 변경된 경로만 실행한다. 같은 실패는 관련 수정이나 새 증거 없이 반복하지 않는다. 고정 토큰 예산을 새로 만들지 않았으며 무제한 비용을 허용한 것도 아니다. 새 동작의 실제 증거가 없으면 Not verified로 남긴다. 마지막 16개 진단만으로 전체 요청의 오류 없음이나 이벤트 미발생을 단정하지 않는다.

원래 목표는 compact 계측·후속 Read·자동 압축·역할/컨텍스트 증거 확인이며 goal 도구에는 마지막 조회 시 blocked로 남아 있었다. 로컬 후속 수정은 진행됐지만 전체 목표는 미완료다. 새 세션에서 get_goal로 현재 상태를 확인하고 사용 가능한 상태 API의 의미를 지켜라. 이관 문서 작성, 목표 예산 소진 또는 부분 성공만으로 완료 표시하지 마라. 목표를 새로 생성해 기존 미완료를 숨기지 않는다.

사용자는 원인별 수정→검증→diff 검토→커밋을 요구했다. 필요한 로컬 작업을 중간 확인 질문으로 끊지 말고, 원인이 확인되면 수정과 검증까지 수행한 뒤 무엇을 고쳤고 어떤 실제 확인이 남았는지 보고하라. 커밋 대상은 해당 변경만 명시하고 원격 push는 하지 않는다. 이 문서도 사용자 확정에 따라 커밋한다.

완료 보고에는 변경·커밋 ID, 실제 실행한 검사와 결과, 실제 사용자 세션 증거, 미지원/미검증/보류를 구분한다. 현황표의 미완료 항목이 남아 있으면 Clauduct 전체 또는 모든 GPT 경로 검증 완료라고 말하지 마라.
