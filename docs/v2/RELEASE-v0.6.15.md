# v0.6.15 — native 2.1.330 인자 판정 재고정

범위는 [#331](https://github.com/wotjr1649/Clauduct/issues/331)의 native 2.1.330 재고정이다. v0.6.14는 native 2.1.325 실행 파일에만 정밀 인자 판정을 켰으므로, 2.1.330에서는 v0.6.13과 같은 보수 경로를 썼다.

## 바뀐 것

- **native 2.1.330에서 background·관리 명령 인자 판정을 native와 맞춘다.** 실행 파일의 SHA-256이 내장 표의 2.1.330 값과 일치하면, 옵션 값 자리의 `--bg`(예: `--name --bg`)와 옵션 값으로 소비되지 않은 첫 `--` 뒤의 `--bg`는 foreground로 처리한다. 허용된 옵션 뒤의 관리 명령(`clauduct --verbose rm <id>` 등)과 선두 `kill`은 native로 바로 보낸다. 2.1.330에서 새로 생긴 값 옵션 `--initialize-fields`·`--metadata`와 불리언 `--hold-repository-while-parked`의 경계도 반영했다. 다른 실행 파일에서는 기존 보수 경로를 쓴다. [#331](https://github.com/wotjr1649/Clauduct/issues/331), [ARCHITECTURE §4](ARCHITECTURE.md#4-cli-계약).
- **`--metadata --bg --help`처럼 값 자리에 있는 `--bg`는 도움말 단축 경로를 막지 않는다** (2.1.330 고정 빌드에서만).

## 출하 검사

이번 출하부터 매 출하 관문은 후보에 묶인 출하 검사(GATE-T1)다. 전체 오프라인 Go 테스트, native 대표 시나리오, 바뀐 동작의 실제 backend 표본, 설치·업데이트 검사를 포함한다. 결과는 발행 뒤 이 문서에 기록한다.

## 출하 신원

발행 뒤 채움

## 알려진 제한

- **native가 설정·로그인 상태에 따라 background 요청을 root 경로로 넘기는 분기는 실제로 발동시켜 확인하지 못했다.** Clauduct는 이 경우에도 background 요청으로 판정하고, 실제 처리는 native에 맡긴다. [#331](https://github.com/wotjr1649/Clauduct/issues/331).
- background의 native telemetry·계정 조회와 Clauduct job 종료 시 복원한 resident가 함께 끝날 수 있는 제한은 계속 적용된다. [v0.6.14 알려진 제한](RELEASE-v0.6.14.md#알려진-제한).
