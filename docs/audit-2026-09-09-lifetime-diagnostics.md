# Gateway 수명 누적 진단

완료 복귀 실제 성공 기록 adb4d26 이후의 후속 작업이다. 마지막 16개 요청만으로 전체 실패·이벤트 발생 유무를 판단하던 진단 공백을 줄였다.

gateway의 lifetime과 request-status의 lifetime(scope=gateway-lifetime)에 started, succeeded, failed, auxiliaryMetadataEvents, unsupportedEvents 다섯 정수 카운터를 추가했다. 기존 최근 16개 상세 기록은 유지한다. 출력은 고정 키와 비음수 정수만 허용하며 이전 버전에서 lifetime이 없으면 null로 표시한다.

- started는 메시지 endpoint의 인증·헤더 검사를 거쳐 요청 시간 계측에 들어간 수다. 입장 대기 중인 요청도 포함한다. 계측 전 거부 및 status/models/등록 endpoint는 제외한다.
- succeeded/failed는 계측된 요청의 종료 시 한 번 갱신한다. 진행 중에는 두 수의 합이 started보다 작을 수 있다. 모든 실패가 upstream API 실패인 것은 아니다.
- auxiliaryMetadataEvents는 형식 검증을 통과해 처리된 codex.response.metadata 이벤트 수다. 재시도 중 처리된 이벤트도 센다. 요청 성공 횟수와 같지 않다.
- unsupportedEvents는 계측된 요청이 UNSUPPORTED_EVENT로 실패한 횟수다. 이름·본문을 기록하지 않으며 과거 other의 원래 이름은 복원하지 않는다.
- 수명은 native 세션이 아닌 gateway 프로세스다. /clear 등으로 native 세션이 바뀌어도 유지되고 gateway 재시작 시 0부터 시작한다. 재시작 전 통계는 소급 생성하지 않는다.

onEvent와 legacy 배열 응답에 공통 이벤트 계측을 적용했다. 이전에는 배열 응답의 보조 이벤트가 처리돼도 카운터가 증가하지 않았다. live heartbeat 시작·종료 정책은 그대로 유지한다. 알려진 native 역할 claude를 상태 조회의 고정 어휘에 추가했으며 모델 라우팅은 변경하지 않았다.

Verified: test-native-gateway.mjs 22개, test-completion-selection.mjs 46개, test-agent-selection.mjs 통과. 미지원 이벤트 실패 후 성공 17회를 실행해 해당 실패가 최근 기록에서 사라져도 누적 failed=1/unsupportedEvents=1임을 확인했다. 두 transport 형식의 metadata 이벤트 카운터, snapshot 변경으로 내부 카운터가 바뀌지 않음, 상태 원문 비노출, role=claude 보존을 확인했다. git diff --check와 변경 diff를 검토했다.

Not verified: 새 누적 진단의 실제 사용자 세션 출력, 실제 보조 이벤트 발생. gateway별 불투명 세션/agent 상관관계 ID 및 실패 단계별 누적 분류는 아직 미구현이다. 전체 동적 모델 경로 목록·provider/fallback 전수 검증도 남아 있다. 기존 완료 복귀 테스트를 실제로 다시 실행하지 않았고 인증/live·symlink 거부를 우회하지 않았다.
