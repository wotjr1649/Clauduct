# v0.5.0 — 요청 세션 식별, 역할·fork 범위, 자산 넷

Claude Code 2.1.283·Codex CLI 0.157.0 기준. 개발 후보의 무료 회귀와 실제 backend 검증 결과는
[호환성 문서](COMPATIBILITY.md#v050--요청-세션-식별-역할fork-범위-자산-넷)에 있다. 태그 바이너리의 출하 검사는
발행 후 이 문서에 더한다.

## 바뀐 것

- **Claude Code 2.1.283 인자.** 새 필수값 옵션 `--client-data-url <url>`을 인자 표에 더했다. 이전 빌드는 이 옵션을
  모르는 옵션으로 읽어, 뒤에 온 `--settings`를 `SETTINGS_INVALID`로 거부하고 `--model`·`-p`의 경계를 잃었다.
  서명 구성 문서의 적재·검사는 native가 처리한다(native는 `https://downloads.claude.ai/` 외의 값을 모델 요청 전에 거부).
- **요청 세션 식별(#135).** 모든 생성 요청에 native 세션별 불투명 키를 `prompt_cache_key`와 `session-id` 헤더로
  보낸다. 전송은 HTTP SSE, 본문은 기존 형식 그대로다. 결정 근거와 재검토 조건은
  [호환성 문서](COMPATIBILITY.md#v050--요청-세션-식별-역할fork-범위-자산-넷)에 있다.
- **커스텀 역할 기본값.** `--add-dir`의 역할 정의, `--setting-sources`의 출처 제외, settings의
  `CLAUDE_CODE_SUBAGENT_MODEL`과 그 effort를 native와 같게 읽는다. 이전 빌드는 네 경우에 native와 다른 모델·effort로
  자식을 실행했다(예: 부모 low에서 native sol/low, 이전 빌드 sol/xhigh).
- **subagent 안의 forked Skill.** 검증된 subagent가 부른 `context: fork` skill의 자식을 받아들인다. 이전 빌드는
  `AGENT_SELECTION_UNVERIFIED`로 거부했다. fork 자식의 `SendMessage` 재개를 "거부한다"고 적은 이전 문서도 바로잡았다.
  2.1.283에서 native는 이 재개를 받아들이고, 제품은 같은 자식을 native 영수증과 대조해 실행한다.
- **릴리스 자산 넷(#136).** `clauduct.exe`·`install.ps1`·`uninstall.ps1`·`SHA256SUMS`. v0.4.x가 0.3.x updater를 위해
  함께 올리던 `clauduct-hook.exe`·`clauduct-dev.exe` 사본은 더 이상 올리지 않는다.

## 0.3.x 설치에서 올라오는 경우

0.3.x의 `clauduct --update`는 사본이 없는 v0.5.0을 받지 못하고 다음처럼 멈춘다. 설치는 바뀌지 않는다.

```text
clauduct: release v0.5.0 does not carry clauduct-hook.exe, clauduct-dev.exe
```

[README](../../README.md)의 설치 스크립트로 다시 설치한다. 설치 스크립트가 세 파일을 `clauduct.exe` 하나로 바꾼다.
v0.4.x 설치의 `clauduct --update`는 그대로 동작한다.
