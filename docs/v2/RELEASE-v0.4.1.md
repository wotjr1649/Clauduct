# Clauduct v0.4.1

Windows x64용 Go V2 릴리스 후보다. 두 클라이언트(Claude Code, Codex CLI)가 따로 업데이트되는 것을 따라가는 릴리스다.
버전을 고정하지 않고, 업데이트가 무엇을 바꿨는지 과금 없이 드러나게 한다. 배포 여부와 검사 결과는 해당 후보 commit의
출하 검증 기록으로 확인한다.

- **조용한 응답을 끊기지 않게 한다.** 과금 없이 측정해 보니 Claude Code 2.1.281은 gateway를 거칠 때 바이트가 6분 동안 오지 않으면
  연결을 끊고 재시도했다. 첫 출력 전후와 `-p`·TUI 모두 같았다.
  - 이 빌드는 모델이 생각하는 동안 아무것도 보내지 않는다. 그래서 오래 생각한 턴은 재시도가 replay 차단
    (`NATIVE_REQUEST_REPLAY_BLOCKED`)에 걸려 사라졌다.
  - 이제 첫 출력 뒤에는 30초 무출력마다 SSE `ping`을 보낸다. 첫 출력 전에는 240초 동안 아무것도 쓰지 않았으면 메시지를 열고 ping을 보낸다.
  - 240초 전의 실패는 지금처럼 상태 코드(429·400 등)로 답한다. 그 뒤의 실패는 오류 이벤트로 오며, 원인은 그대로 보인다
    ([#120](https://github.com/wotjr1649/Clauduct/issues/120)).
- **모르는 요청 요소는 이름과 함께 거부한다.** 모르는 필드·키·content block·thinking type은 전처럼 거부한다.
  - 거부한 이름은 세션 상태의 `requests.refusedElements`에 `범주 이름`으로 남는다(처음 8개, 모양 제한).
    클라이언트 업데이트가 요청에 무언가를 더했을 때 무엇인지 바로 알 수 있다.
  - 모르는 `X-Claude-Code-Request-Class` 값은 `INVALID_HEADER` 대신 `REQUEST_CLASS_UNKNOWN`(400)이다
    ([#127](https://github.com/wotjr1649/Clauduct/issues/127)).
- **측정하지 않은 클라이언트를 알린다.**
  - 설치된 클라이언트가 마지막으로 측정한 버전과 다르면, 종료 줄에 `unmeasured=claude/<버전>,codex/<버전>`이 붙는다.
  - `clauduct --dev --doctor`는 두 클라이언트의 설치 버전과 측정 버전, Codex 경로, Codex 로그인의 사용 가능 여부를 보인다.
    로그인은 범주나 만료 시각만 보이고 토큰은 보이지 않는다.
  - 세션 상태에 `gateway.codex`(요청에 실린 Codex 버전과 판정)가 추가됐다. 세션은 버전 때문에 막히지 않는다.
- **Codex CLI 기준을 0.156.1로 옮겼다.** 설치된 0.156.1의 요청을 과금 없이 캡처해 이 빌드의 요청과 비교했다.
  - 0.156.1은 WebSocket을 먼저 쓰고, 도구를 입력 항목으로 싣는 새 모양을 쓴다.
  - 이 빌드는 요청 모양을 바꾸지 않았다. 0.156.1에서 실제 backend를 통과한 모양이다. 새 모양을 따를지는 v0.5.0에서 정한다
    ([#121](https://github.com/wotjr1649/Clauduct/issues/121)).

기준 클라이언트는 Claude Code 2.1.281, Codex CLI 0.156.1이다. 모델 표와 위임 메뉴는 v0.4.0과 같다.
세션 상태 파일에는 필드만 더해졌다.

v0.4.x 동안의 전환은 v0.4.0과 같다: 릴리스는 `clauduct-hook.exe`와 `clauduct-dev.exe`를 `clauduct.exe`의 바이트 동일 사본으로 함께 올린다.
v0.5.0부터는 올리지 않는다. 설치와 업데이트는 [패키징 안내](PACKAGING.md)를 따른다.

**되돌리기.** `install.ps1 -Tag v0.4.0`. v0.4.1이 남기는 기록 가운데 v0.4.0이 읽지 못하는 것은 없다.

Windows 전용이며 서명하지 않는다. 지원 기능과 남은 조건은 [호환성 문서](COMPATIBILITY.md)의 v0.4.1 절을 참조한다.
