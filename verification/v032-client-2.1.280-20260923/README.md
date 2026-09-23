# Claude Code 2.1.280 재측정 (#69)

2026-09-23. v0.3.1의 규칙은 Claude Code 2.1.278에서 측정했고, 설치본은 그 뒤 2.1.280으로 바뀌었다.
[#69](https://github.com/wotjr1649/Clauduct/issues/69)의 조건대로 `--help`를 인자 표와 대조하고,
native 검사를 2.1.280에서 다시 돌린 뒤 통과한 것만 갱신했다. 실제 backend와 실제 TUI는 쓰지 않았다.
모델 호출은 0회다.

## native

검사가 `platform.Resolver`로 찾는 `~/.local/bin/claude.exe`와 `~/.local/share/claude/versions/2.1.280`은
SHA-256 앞 16자가 `0e4195524b73eb77`로 같다. 두 파일의 수정 시각은 2026-09-23 01:58(KST)이고,
`claude --version`은 `2.1.280 (Claude Code)`다. 비교 대상 2.1.278은 `versions/2.1.278`을 직접 실행했다.

## `--help`와 인자 표

두 버전의 `--help` 차이는 `--bare`와 `--safe-mode` 설명 문구 세 곳이다([차이](help.diff)).
`Options:` 절의 옵션 65줄은 이름과 값 형태(`<...>` 필수, `[...]` 선택, 없음)가 같다.
설치 native의 `--help`를 [인자 표](../../go/internal/app/native_args.go)와 대조하는
`TestNativePublicOptionAritiesMatchScanner`는 2.1.280에서 63개를 확인하고 통과했다. 나머지 2개는
권한 우회 옵션이며, [launch 거부](../../go/internal/launch/refuse.go)가 먼저 막으므로 표에 없다.

## plugin API

native 이벤트 모듈(`go/internal/app/native-events.mjs`)은 `--help`에 없는 plugin API를 쓴다.
두 버전에서 `/plugin-types`로 선언 파일을 받았다. [임시 검사](plugin-types-probe_test.go)를 overlay로
붙여 fixture backend와 함께 실행했고 backend 호출은 0회였다.

| native | bytes | SHA-256 |
|---|---|---|
| 2.1.278 | 504,976 | `95bc0d31b9b5939c9eb2b4c68eb5aee36f01e50403eca234bbc749ccb73bcd88` |
| 2.1.280 | 549,335 | `863f6431a0e456e51aa72bb40b79108025e3cde67d846b01abd8e3b037de9396` |

선언 전체는 달라졌다. 모듈이 쓰는 이벤트(`turn.step`, `turn.complete`, `turn.start`, `prompt.submit`,
`session.start`, `tool.call`)의 입력·결과 타입과 `$.session.id`, `$.clock.now`, `$.fs`, `$.tool.call`의
시그니처만 [추출기](extract-types.cjs)로 뽑아 비교했다([차이](plugin-api.diff)). 달라진 것은
`ToolCallResult`의 선택 필드 `isReadOnly` 추가와 `TurnUsage`의 기반 타입 이름뿐이다. 모듈은 usage를
읽지 않고 그대로 넘긴다. 선언 파일은 저장소에 두지 않는다(`native_api_evidence_test.go`와 같은 이유).

## native 검사

Go 1.27.1. 변경 후 일반(`CGO_ENABLED=0`)·race 전체가 19 package 모두 통과했다. 일반 실행의 최상위 검사는
824 통과, 3 SKIP이다. 결과는 [기록](go-test.txt)에 있다. evidence 태그 중 스위치 없이 fixture로만 native를
쓰는 검사 6개(`-tags "runtime_evidence policy_evidence"`)는 변경 전 코드로 2.1.280에서 통과했다. 변경은
상수와 주석뿐이며, 그중 CI가 돌리는 3개는 변경 후에도 통과했다.

첫 실행은 변경 전 코드였고, `internal/app`만 돌렸다. 이 PC의 기본값이 `CGO_ENABLED=1`이어서
`TestTheShippedBinaryIsBuiltWithoutCgo`가 실패했다. `CGO_ENABLED=0`으로 그 검사만 다시 돌려 통과했다.
나머지 179개는 통과했고 SKIP 3개는 native와 무관한 스위치 검사다.

## 갱신한 것

- `ReferenceClient`: `2.1.278` → `2.1.280`. 진단의 `verified`는 버전 문자열 일치만 뜻한다.
- 인자 표 주석, `delegation.go`의 `subagent_type` 생략 주석. 생략 경로는
  `TestNativeOmittedChildRoleRetainsVerifiedInheritance`가 2.1.280에서 확인했다.
- `docs/v2/ARCHITECTURE.md`의 옵션 형태 기준, `docs/v2/README.md`의 측정된 클라이언트.
- `docs/v2/COMPATIBILITY.md` 0절: 재측정 행을 추가했다. "최근 실제 TUI" 행은 2.1.278로 둔다.
  같은 문서의 2.1.278 관측 기록(묶음 1 검사, 혼합 PDF, 첫 본문 표시)도 그 버전의 근거로 둔다.

## v0.3.1 기록 정정

[집중 리뷰 기록](../v031-focused-review-20260923/README.md)은 native를 2.1.278로 적었다. 그 리뷰의
대상 `99b681f`는 2026-09-23 09:33(KST)에 커밋됐다. 그때 검사가 실행하는 `claude.exe`는 이미 위의
2.1.280 파일이었다. 기록을 2.1.280으로 고쳤다.

## 남은 것

- 2.1.280 실제 TUI는 실행하지 않았다. PTY가 필요하고 `winpty`는 이 세션에서 쓸 수 없다.
- plugin API는 모듈이 쓰는 부분만 비교했다. 선언의 다른 변화가 제품에 영향을 주는지는 보지 않았다.
