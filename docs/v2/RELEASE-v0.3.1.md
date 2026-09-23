# Clauduct v0.3.1

Windows x64용 Go V2 유지보수 릴리스 후보다. 배포 여부와 검사 결과는 해당 후보 commit의
출하 검증 기록으로 확인한다. Node V1은 비교 기준으로 유지한다.

- native 인자와 사용자 settings의 값 경계를 보존하고, 역할·model·effort 선택을 검증한다.
- 세션·turn 영수증, 압축 journal 복원, projects 경로와 종료 시 정리를 보완한다.
- HTTP 응답 종료 시 제한된 drain을 적용하고, 불확실한 요청의 자동 중복 실행을 거부한다.
- 내장 `httpguard`는 OS와 무관한 표준 Go로 파서 오류 응답의 길이를 지정하고 연결 종료를 처리한다.
  실행 전 취소된 요청은 backend를 호출하거나 재전송 방지 예약을 소비하지 않는다.
  필터에 남은 미완성 본문은 검증된 같은 turn의 재연결에서 정리한다.
- native가 오류·거부·취소로 종료한 turn의 기록을 Go에서 대조하여 남은 추론·검색 요청을
  정리한다. 이전 turn의 기록은 새 turn을 취소하지 않는다. 복구 범위는 실행 전 재연결,
  실행 여부가 불확실한 요청의 중복 방지, 같은 native 프로세스의 후속 작업이다.
  완료됐지만 전달되지 못한 응답의 보관·자동 재전달은 포함하지 않는다.
- Agent·Workflow의 빈 대기 응답과 완료 알림을 구분한다. SDK의 출처 표시 상태는
  자식 보고서나 모델의 작업 완료 답변을 대신하지 않는다.
- `/clear` 뒤의 요청이 거부되던 문제를 고쳤다. native는 같은 프로세스에서 새 세션을
  시작하는데, 영수증이 처음 세션 ID를 계속 기록해 v0.3.0에서도 `PARENT_WAIT_UNVERIFIED`로
  실패했다. 이제 영수증마다 현재 세션 ID를 기록한다.
- 한 native 프로세스가 turn·자식·취소를 합쳐 4096개 넘게 게시하면 이후 요청이
  `CLAUDUCT_NATIVE_EVENT_LIMIT`로 멈추던 v0.3.0의 한도를 없앴다. 끝난 turn은 자리를 반환하고,
  한도는 동시에 진행 중인 agent 수에만 남는다. gateway는 지난 영수증 파일을 정리한다.
  요청 실행 기록의 세션당 16,384개 한도는 그대로이며 [#63](https://github.com/wotjr1649/Clauduct/issues/63)에서 다룬다.
  [집중 리뷰 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-focused-review-20260923/README.md)

출하 구성은 Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`다. `clauduct.exe`,
`clauduct-hook.exe`, `clauduct-dev.exe` 세 파일과 `SHA256SUMS`를 함께 사용한다.
설치와 업데이트는 [패키징 안내](PACKAGING.md)를 따른다.

**v0.3.0으로 되돌릴 때 기존 세션 재개에는 제한이 있다.** v0.3.1이 저장한 선택 journal의
`customRole` 필드 또는 `native-selection` 출처는 v0.3.0 reader가 거부한다.
내장 역할의 저장 철자와 native metadata의 대소문자가 다를 때도 구버전의 정확 비교가 거부한다.
해당 세션은 v0.3.1로 계속하거나, v0.3.0에서 새 세션을 시작한다. 바이너리 교체는 세션
기록을 변환하지 않는다. 기록을 삭제하거나 필드를 제거해서 재개 검사를 통과시키지 않는다.

AdGuard for Windows에서 응답이 유실되면 전체 보호는 유지한 채 로컬 호스트 필터링만 끄고
다시 확인한다. 8.0.5570에서 확인한 범위와 다른 필터 제품의 확인 방법은
[전후 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-localhost-filter-20260923/README.md)과 호환성 문서에 있다.

Windows 전용이며 서명하지 않는다. 지원 기능과 남은 조건은
[호환성 문서](COMPATIBILITY.md), 수정 검토는
[batch-04 보고서](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-review-fixes-20260922/batch-04/REPORT.md)와
[내장 필터 검증](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-httpguard-20260923/REPORT.md)을 참조한다.
