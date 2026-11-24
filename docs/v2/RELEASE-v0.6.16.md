# v0.6.16 — native 인자 판정을 버전과 무관하게 단순화

범위는 native 인자 판정의 버전 고정 제거와 PDF 페이지 Read 안내다. native는 주마다 여러 번 갱신되므로 특정 실행 파일에 묶은 판정을 유지하지 않는다.

## 바뀐 것

- **모든 native에서 같은 인자 판정을 쓴다.** argv 어디든 정확한 `--bg`·`--background` 토큰이 있으면 background 요청으로 판정한다. 옵션 값 자리(`--name --bg`)나 `--` 뒤의 `--bg`도 background 요청이 된다. 이 토큰이 없으면 선두 `rm`·`stop`·`logs`만 관리 명령 후보다. 선행 옵션 뒤 관리 명령과 선두 `kill`은 세션 경로를 탄다. v0.6.15까지 native 2.1.330 실행 파일에만 적용하던 정밀 판정 표를 제거했다. [#331](https://github.com/wotjr1649/Clauduct/issues/331).
- **긴 PDF 페이지 Read에 필요한 Poppler 설치 방법을 안내한다.** [README](../../README.md), [COMPATIBILITY](COMPATIBILITY.md).

## 출하 검사

결과는 발행 뒤 이 문서에 기록한다.

## 출하 신원

발행 뒤 채움

## 알려진 제한

- **옵션 값이나 `--` 뒤에 `--bg`를 쓰면 background 요청으로 판정한다.** native는 `--name --bg`·`-- --bg`를 foreground로 처리하므로 두 판단이 갈려 `BACKGROUND_DISPATCH_UNVERIFIED`로 실패한다. 이름이나 프롬프트에 `--bg`라는 문자열을 단독 인자로 쓰지 않으면 영향이 없다.- **10쪽이 넘는 PDF를 페이지 지정으로 읽으려면 Poppler(`pdftoppm`)가 필요하다.** native Claude Code와 같은 동작이다.
- background의 native telemetry·계정 조회와 Clauduct job 종료 시 복원한 resident가 함께 끝날 수 있는 제한은 계속 적용된다. [v0.6.14 알려진 제한](RELEASE-v0.6.14.md#알려진-제한).
