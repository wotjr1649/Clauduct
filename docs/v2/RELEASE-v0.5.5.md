# v0.5.5 — PowerShell 7·phase 보존·분석 경고 정리

v0.5.5는 v0.5.4 감사에서 남은 항목을 보완한다. 묶음 C의 새 기능은 포함하지 않는다.
출하 완료 신원과 설치 결과는 최종 자산 검증 후 이 문서에 기록한다.

- 설치·제거·Windows 검증 스크립트의 최소 버전을 PowerShell 7로 통일했다. 5.1 지원은 종료한다.
  5.1 호환용 hash·TLS 코드를 표준 PowerShell 7 명령으로 줄였으며 실행 정책을 변경하지 않는다.
- backend assistant 메시지의 `commentary`·`final_answer` phase를 native text block에 보존한다.
  스트리밍·대화 저장·UUID 재개·fork 후 원래 phase를 다시 backend에 보낸다. 없는 값을 추정하지 않고
  user/system/tool-result의 phase나 중간에 바뀐 phase는 거부한다.
- text block의 `phase`는 Anthropic 표준 밖의 Clauduct 확장이다. **이 필드가 포함된 새 대화는
  v0.5.5 이상에서 재개해야 한다.** v0.5.3/4는 명시적으로 거부하며 설정·snapshot·기존 이력은 보존한다.
  v0.5.5로 복귀하면 같은 UUID로 이어갈 수 있다. 기존 phase 없는 대화는 그대로 읽는다.
- 이미지/PDF의 공통 검증을 통합하고 쓰이지 않는 반환값·매개변수와 스타일 경고를 정리했다.
  미디어 허용목록·오류 분류·native PDF 오류 문구는 유지한다. 같은 uncapped 분석 설정에서
  dupl 2·staticcheck 62·unparam 16의 **80건을 0건**으로 줄였다. unused도 0건이다.
- native와 Clauduct가 담당하는 부분을 [아키텍처](ARCHITECTURE.md)에 구분하고, v0.5.4의
  중간 빌드에서 얻은 중복 차단 증거를 최종 바이너리의 증거와 구분했다.

초기 settings 생성·기존 파일 보존·S 선택·모델 매핑은 v0.5.4의 계약을 유지한다.
B의 native 분류기·추가 규칙·권한 설정도 유지한다. 지원 범위와 미검증 경계는
[호환성 문서](COMPATIBILITY.md), 설치 및 되돌리기는 [패키징 문서](PACKAGING.md)를 따른다.

로컬 전체 일반·race는 각각 22개 패키지, gofmt·vet·build·문서 인용과 uncapped 분석이 통과했다.
phase 해독·이력 재전송·snapshot 대조와 media 허용목록·PDF 문구·credential redaction 수정을
되돌린 검사는 실패했다. Sol/medium·Luna/medium·Terra/high의 실제 phase 왕복과 backend 계수도
통과했다. 후보 바이너리에서 전체 지침을 유지한 중복 차단 발동과 별도 정상 표본을 확인했다.
이 후보 증거를 최종 태그 바이너리의 검사로 대체하지 않는다.
