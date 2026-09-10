# 요청 검증 진단과 Windows 테스트 실행 환경

## 배경과 판정

88616a2d-651f-4df5-84cb-41beca26bb31에서는 구현 자식이 test/untag.test.mjs를 작성하고 5개 RED를 관측한 뒤 다음 모델 요청이 HTTP 400 UNSUPPORTED_REQUEST로 종료됐다. 비동기 failed 알림은 메인에 전달됐다. 원본 요청 payload와 세부 거부 위치가 보존되지 않아 정확한 실패 필드는 아직 미확정이다. 이번 변경은 그 원인 해결을 선언하지 않고 다음 발생을 안전하게 식별하도록 한다.

## 요청 진단 변경

- 기존 요청 허용 필드, 검증 조건, 오류 category와 HTTP 상태를 유지한다. 실패에 requestFailure 고정 라벨을 추가한다. REQUEST_FIELDS, THINKING_TYPE, TOOL_RESULT_FIELDS 등 위치/조건만 구분하며 원본 필드 이름·값·본문은 저장하지 않는다.
- gateway의 prepare 단계 UNSUPPORTED_REQUEST에만 허용 목록으로 라벨을 전달한다. 오류 메시지 예: `UNSUPPORTED_REQUEST request=TOOL_RESULT_FIELDS`. request-status의 requestFailure에도 별도 허용 목록을 적용한다. upstream에서 같은 라벨을 주입해도 요청 진단으로 취급하지 않는다.
- 전체 요청·세션·reasoning을 수집하는 로깅은 추가하지 않았다. 기존 최근 16개 진단 보존 정책을 유지한다. 기존 generic INVALID_REQUEST나 별도 오류 category 전부를 새 라벨로 전환한 것은 아니다.

## 공통 테스트 실행기

`src/run-node-tests.ps1`은 Windows의 PowerShell 7 현재 셸에서 직접 실행한다. 다른 셸을 띄우거나 execution policy를 변경하지 않는다. 메인과 구현 자식 모두 이 실행 경로를 호스트가 허용할 때 재사용할 수 있다. native Bash에서의 실제 호출 허용 여부는 아직 미검증이며 guard 거부를 우회하면 안 된다.

```powershell
& D:\AIDEV\Clauduct\src\run-node-tests.ps1 -Root D:\AIDEV\Clauduct\verification\dev-sandbox\run-02
```

특정 RED만 확인하려면 `-TestFiles test/untag.test.mjs`를 지정한다. 실행기는 선택한 .mjs를 루트 내부에서 확인하고 절대 경로·상위 이탈·reparse point를 거부한다. 검토된 테스트 코드만 실행해야 한다.

- Node는 shell=false의 직접 자식 프로세스다. 환경은 허용 목록으로 새로 구성한다. SystemRoot/WINDIR은 Windows SpecialFolder에서 얻은 canonical 경로이며 Bash 변수 대소문자 확장에 의존하지 않는다. TEMP/TMP는 작업 루트의 .tmp이고 나머지 부모 환경은 전달하지 않는다.
- 처음부터 `--test --test-concurrency=1`을 사용한다. 전체 기본 timeout 60초, 허용 설정 1~60초. timeout은 자식 process tree 종료 후 exit 124, 테스트 실패는 원래 test runner exit code를 보존한다.
- stdout/stderr는 읽은 뒤 합계 2 MiB 초과를 오류 처리한다. 이는 보고 상한이며 스트림 메모리 상한이 아니다. OS 네트워크/파일 격리나 악성 테스트의 비밀 출력 방지 장치도 아니다. 승인된 테스트의 기존 임시 디렉터리·자식 실행 제한과 함께 사용한다.
- 실행기 테스트에서 만든 runner-fixture-*만 경계를 검증한 뒤 삭제한다. 기존 사용자 파일과 untag RED는 수정·삭제하지 않는다.

## 관측한 검증

| 검사 | 실제 결과 |
|---|---|
| 새 요청 진단 RED | 기존 구현에서 requestFailure 미존재로 실패 |
| test-request-diagnostics.mjs | 24개 통과: 고정 라벨, 정상 is_error tool result 수락, 실제 loopback HTTP 400·status 전달, upstream 미호출·비밀 표식 비노출 |
| test-native-protocol.mjs | 통과 |
| test-native-gateway.mjs | 43개 통과, 비허용 라벨 제거·upstream 오귀속 방지 포함 |
| test-run-node-tests.ps1 | 환경/Windows crypto 3개, 잘못된 경로 3개, 실패 코드 보존, 1초 timeout 처리 통과 |
| run-02 기존 5개 테스트 파일 | 36/36 통과 |
| run-02 전체, untag RED 포함 | 41개 중 36 pass / 5 fail, exit 1. 미구현 untag의 기존 RED이며 숨기거나 제외해 전체 통과로 표시하지 않음 |

요청 진단 검사는 `node --permission --allow-fs-read=D:/AIDEV/Clauduct src/test-request-diagnostics.mjs`로 실행했고 protocol/gateway도 같은 읽기 제한으로 실행했다. 테스트 runner 자체는 실제 Node 실행을 검사하므로 별도 PowerShell 직접 호출을 사용했다. 외부 서비스·실제 인증 모델 호출은 없다.

실행기 개발 중 PowerShell 문법 오류 1건을 수정한 뒤 재검증했다. 셸 경로 확인을 포함한 명령이 ps-nested-shell guard에 거부되어 중첩 셸 호출 경로는 사용하지 않았다. 대신 동일 PowerShell의 스크립트 직접 호출만 검증했다. guard·전역 설정 변경은 없다.

## 남은 범위

- 원본 UNSUPPORTED_REQUEST의 정확한 원인과 변경 후 native 실제 오류 진단 전달은 미검증이다. 새 gateway 프로세스에서 이후 실패가 발생하면 메시지의 request= 라벨 또는 허용된 request-status로 판정한다.
- 원본 Node CSPRNG assertion의 정확한 환경 변수 원인은 복원하지 못했다. 새 실행기의 canonical Windows 환경에서 초기화가 성공했다는 증거와 구분한다.
- native SDD 구현→검토→통합 완주, 실제 model/effort routing, symlink/junction 동적 시험, 장기 자원 누적은 이번 변경의 완료 범위가 아니다.
- run-02 HEAD는 298a533이고 test/untag.test.mjs가 untracked 상태로 남아 있다. session-16의 clean 사전 조건을 그대로 재사용하면 안 된다. 후속 시험은 보존된 RED와 새 실행기를 명시적으로 이어받아야 한다.
