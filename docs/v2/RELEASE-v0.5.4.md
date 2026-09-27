# v0.5.4 — 초기 설정 완성 및 background 결과 보존

2026-09-28 [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.5.4)를 발행하고 실제 설치를 완료했다.
태그는 `58f9491540d6a7f6ebe9f4d74787a829d114cd63`이며 출하 바이너리의 SHA-256은
`ce9376872ae13e97f0b449681c02c7bcc668b07f6ed140b212937a5b9c9ba8e7`이다.

v0.5.3에서 초기 파일이 `{"version":1}`로 생성되어 편집할 모델·effort·agent가 드러나지 않았던
누락을 보완한다. 파일이 없으면 `startup`, `modelDefaults`, `modelMapping`, `agents`를 모두 담은
[기본 문서](../../go/internal/settingsfile/defaults.json)를 생성한다. 설치와 runtime의 생략값 처리는
이 원본을 공유한다. 기존 파일은 내용과 버전에 관계없이 자동 덮어쓰기·병합하지 않는다.

새 실행의 시작값은 Sol/xhigh이며 GPT 공통 effort와 개별 agent pair는 각각 독립적이다.
S 선택은 현재 세션에 저장하고 수동 settings 파일은 변경하지 않는다. 설정 우선순위와 재개 지원
범위는 [SETTINGS.md](SETTINGS.md)에 정리했다. 시작 model과 effort의 출처를 상태 JSON에 각각 추가했다.

background worker가 같은 세션으로 respawn할 때 마지막 선택이 기존 시작값과 일치하면 원래
설정 snapshot으로 재연결한다. worker 실행 중 settings 파일을 편집해도 그 세션의 부모와 새 자식
선택을 바꾸지 않는다. S로 다른 선택을 저장한 worker에는 정확한 UUID 재개 명령을 안내한다.

실제 background 검증에서 추가로 드러난 handback 종료 문제를 고쳤다. native가 보고서를
전달한 뒤 자식이 빈 응답으로 종료할 때 `EMPTY_REPLY` 또는 재전송 차단으로 바뀌던 경로다.
같은 자식·turn·call의 native 성공 증거를 대조한 경우에만 종료 상태로 처리하고, 전달한 보고서는
후속 closing text로 지우거나 교체하지 않는다. native classifier와 B의 추가 규칙은 유지한다.

Agent 이력에서 원래 모델·effort 선택을 복원하는 경계도 보완했다. native spawn의 GPT ID와
도구 인자의 Claude 호환 alias가 다른 경우에도 검증된 호출의 원본 선택을 복원한다.
다른 세션·부모·호출·역할·모델의 이력은 수정하지 않는다.

이미 수락되어 결과를 기다리는 Agent와 인자가 모두 같은 후속 호출은 새 실행으로 전달하지 않고
기존 native 대기로 돌린다. 같은 세션·부모·현재 turn의 수락 이력과 대기 상태를 확인하며, backend
응답을 기다리는 동안 call ID가 자식 ID로 연결되는 경우도 같은 선택 기록으로 대조한다.
원본 model/effort의 생략·명시 여부를 보존하고 최상위 JSON 키 순서와 문서 공백만 정규화한다.
프롬프트 내용의 유사성은 판단하지 않는다. 인자의 지문은 프로세스 메모리에만 유지한다.

최초 응답 안의 병렬 호출, 다른 인자, 새 사용자 요청, 이미 결과를 받은 뒤의 위임은 유지한다.
같은 인자의 작업을 한 요청에서 순차적으로 따로 시작하려면 description 등을 구분해야 한다.
중복과 다른 도구가 섞인 응답은 일부만 실행하지 않고 거부한다. 상태 JSON의
`parentReadiness.duplicateAgentCalls`는 실제 대기로 전환한 호출 수이며 작업 성공 판정이 아니다.
출력 stop sequence가 제외한 도구는 실행 준비도 하지 않는다.

Agent 도구 설명에는 현재 assistant turn을 텍스트로 끝내고 native 완료 알림을 기다리는 안내를
유지한다. 설명만으로 동일 호출을 막을 수 없어 위 제어를 보완했으며, 설명 제거 대조에서는 필수
prompt가 없는 추가 Agent 호출이 관측됐다. native 전역 지침·B 안전 규칙·도구 권한은 변경하지 않는다.
실제 backend의 동일 호출을 한 번 차단하고 원래 보고서와 최종 답변으로 정상 종료한 표본을
확인했다. 최종 조합의 Luna/medium 일반·독립 표본과 Sol/xhigh background 재시작 일반·독립 표본도
통과했다. 최초 출하 후보의 Luna 중복 실행 실패와 설명 제거 대조의 필수 prompt 누락은 실패로
유지한다. 후자의 리뷰에서 발견한 검사기 누락도 고쳐 native 도구 실패 0을 필수로 검사한다.

최초 감사에서 남은 미사용 선언 4개와 테스트 전용 제품 파일 2개를 제거했다.
환경변수 변환 통합은 이전 v0.5.3의 변경이며 이번 정리량으로 다시 세지 않는다.
새 의존성이나 별도 설정 프레임워크는 추가하지 않았다.
상한 없이 수행한 정적 분석은 unused 0, dupl 2, staticcheck 62, unparam 16으로 총 80건·exit 1이다.
SA 오류는 없지만 기존 스타일·공유 반환 계약 경고와 정책이 다른 이미지/PDF 유사 코드는 남아 있다.
전체 분석기가 통과했다거나 모든 의미상 중복을 제거했다고 주장하지 않는다.

설치·되돌림의 범위와 PowerShell 5.1 실행 미검증은 [PACKAGING.md](PACKAGING.md#6-되돌리기)에 있다.
v0.5.3과 v0.5.2의 실제 바이너리로 설정·snapshot·S 선택·Agent 이력이 있는 대화를 되돌렸다가
현재 코드로 복귀하는 검사를 수행했다. v0.5.3은 선택을 복원했고 v0.5.2는 이전 기본값으로 열었으며,
둘 다 원래 파일과 이력을 보존했다. 구버전의 유료 backend 응답을 검증한 것은 아니다.

최종 전체 일반·race는 각각 22개 패키지가 통과했고 gofmt·vet·build·문서 인용 검사도 통과했다.
동일 호출 제어, call ID에서 자식 ID로의 연결, stop sequence 뒤 실행 준비, 진단 지문 제외를 각각
제거한 네 대조에서는 해당 assertion이 실패했다. 수정이 없는 코드도 통과하는 검사로 대신하지 않았다.

개발·실패·취소·재시도·출하를 합한 실제 backend 사용량은 **532/승인 2000 attempts**다.
Go 1.27.1·CGO=0·trimpath의 깨끗한 태그를 독립 빌드 캐시 두 개로 빌드해 바이트 일치를 확인했다.
그 설치본의 입력·clear·Agent 위임은 5회로 통과했으며 위 합계에 포함한다.
자산 4개(`clauduct.exe`, `install.ps1`, `uninstall.ps1`, `SHA256SUMS`)의 공개 digest와 다운로드
바이트를 대조했다. 신규 설치, v0.5.3의 공개 updater, 이미 최신인 경우의 무변경, v0.5.3 되돌림,
v0.3.5 updater의 안전한 거부와 새 installer 전환도 통과했다.

실제 설치본의 버전·commit·hash를 대조하고 기존 사용자 settings 파일과 PATH가 보존됨을 확인했다.
측정된 클라이언트는 Claude Code 2.1.283과 Codex CLI 0.157.1이다. PowerShell 7 실행 검사는 PASS이며,
Windows PowerShell 5.1은 AST 검사 PASS/실행 NOT_RUN이다. 실행 정책을 우회하지 않았다.
묶음 C의 Workflow 확장·Office·전체 기능 최종 인수는 이 릴리스에 포함하지 않는다.
