# v0.6.x 후속 품질 검증

현재 Windows 지원 범위를 검증하고 한계를 명시한다. 기존 미지원 기능을 확대하거나 새 버전의 출시를
선언하는 문서가 아니다. 출시·설치 상태는 [현재 상태](README.md), 기능 계약은 [호환성](COMPATIBILITY.md)이 소유한다.

## 우선순위와 완료 경계

| 작업 | 범위 | 완료 경계 |
|---|---|---|
| [후속 패치 출하 #198](https://github.com/wotjr1649/Clauduct/issues/198) | Agent 종료 수정 등 승인된 후속 변경 전달 | 별도 출하 지시와 최종 자산·설치 검증. 개발 PR 병합만으로 완료하지 않음 |
| [벤치마크 #199](https://github.com/wotjr1649/Clauduct/issues/199) | native 이벤트 경로와 합성 모델 metadata 일치, 반복 성능 측정 | 회귀 검출·로컬 측정·리뷰·병합 후 확인 |
| [자원 #200](https://github.com/wotjr1649/Clauduct/issues/200) | 로컬 반복 부하와 실제 바이너리의 두 동시 세션 | 표본별 한도·프로세스 자원·실제 결과·종료와 예약 반환 |
| [native UI·CLI #201](https://github.com/wotjr1649/Clauduct/issues/201) | 조회 화면, effort 변경·UUID 재개, 명시적 CLI 식별자 | native의 현행 동작과 설정 보존을 독립 표본에서 확인 |
| [복합 Office #202](https://github.com/wotjr1649/Clauduct/issues/202) | 세 형식의 한 곳 편집과 비대상 내용 보존 | 실제 도구 결과와 별도 ZIP/XML·관계·media·reopen 검사 |
| [범위·문서 #203](https://github.com/wotjr1649/Clauduct/issues/203) | 구현·출시·미검증 상태의 구분 | 실제 근거와 잔여 항목 대조 |

Issue를 닫았다는 사실은 모든 기능의 무결함을 뜻하지 않는다. 각 Issue의 완료 조건과 관측 결과를
대조하고, 출하 대기·원인 미확정·실행하지 않은 경로를 남긴다.

## 성능과 자원

2026-09-29의 Windows·Go 1.27.1 로컬 gateway 벤치마크는 각 조건을 20회씩 3번 실행했다.
다음 시간은 회차별 평균의 중앙값이며, backend 추론 없이 localhost 통신·gateway 처리·상태 조정 비용을 포함한다.

| 합성 자식 상태 — 실행·완료 각각 | 입력 | 처리 시간 | 요청당 할당 누계 |
|---|---:|---:|---:|
| 0개 | 228 KiB | 14.92 ms | 약 10.6 MiB |
| 3개 | 236 KiB | 15.51 ms | 약 11.3 MiB |
| 30개 | 310 KiB | 21.52 ms | 약 16.1 MiB |

종료 이력 10·1,000·10,000개에서 정리 루틴의 중앙값은 각각 42.93·41.74·43.89 µs였다.
이 측정은 네트워크 지연·모델 응답 시간·p95/p99·버전 간 성능 무저하의 근거가 아니다.
할당 누계는 최고 점유량이나 누수량과 다르다. 처음 실패한 벤치마크 기록과 수정한 준비 데이터를 구분한다.

로컬 자원 검사는 실제 HTTP·본문 해독·SSE 경로에 1 MiB 입력·256 KiB 출력을 동시성 8로 384회 적용한다.
이때 고정 전송 응답은 gateway 비용을 분리하기 위한 것이며 실제 backend 검증을 대신하지 않는다.
준비 측정에서 각 64회 묶음 뒤 goroutine 수는 27개였고 예약은 반환됐다. GC 뒤 살아 있는 객체와 garbage를
포함한 heap은 약 4–9 MiB, working set은 약 141–200 MiB였다. 짧은 실행에서 working set이 늘었다는 사실만으로
누수라고 판정하거나, heap이 줄었다는 사실만으로 장시간 누수가 없다고 판정하지 않는다.

실제 바이너리의 최종 자원 표본은 `gpt-6-sol/low`와 `gpt-6-luna/low`의 별도 합성 세션을 동시에 유지한다.
각 30턴·턴 간 20초 이상, 큰 합성 입력과 연속 정수 출력을 포함하고 소유한 프로세스의 working set·private bytes·
handle·thread를 관측한다. 요청·전체 실행 시간과 메모리 중단 조건을 고정하며 결과·종료·예약 반환을 각각 판정한다.
별도 취소 표본은 실제 요청과 예약이 있는 상태에서 Ctrl+C를 보내 사용자 취소 분류·native 회수·예약 반환을 확인한다.
정확한 바이너리 신원·관측치·실패 및 잔여 조건은 #200의 완료 근거에 연결한다.

이는 약 10분 규모의 유한 표본이다. 하루 단위 실행, 모든 입력, 여러 프로세스의 메모리 합산 상한을 보장하지 않는다.
프로세스별 수용 예산과 실제 RSS의 차이는 [메모리 정책](COMPATIBILITY.md#v042--메모리-수용-제어)을 따른다.

## native와 Office의 직접 검증

`--print`를 실제 식별자로 실행한다. snapshot이 있는 세션의 `--continue`는 backend에 보내기 전에 UUID
재시작을 안내하는 기존 [재개 계약](SETTINGS.md)을 검증하고, 안내된 `--resume <UUID>`로 내용·선택의 복원을 확인한다.
이 거부를 일반 재개 성공으로 기대했던 준비 검사는 FAIL로 보존한다.

`/effort`는 slider의 `s`로 세션에만 적용한다. 실제 후속 요청과 UUID 재개의 effort, Clauduct·native 설정 파일의
보존을 대조한다. native의 기본값 저장과 세션 전용 선택은 [공식 effort 문서](https://code.claude.com/docs/en/model-config#adjust-effort-level)를 따른다.
`/context`의 backend 계수와 본문 생성을 분리하며, 조회가 항상 backend 호출 0이라는 뜻으로 설명하지 않는다.
Claude Code 2.1.283·2.1.284의 `/agents`는 마법사 제거 안내를 표시한다. `/list-agents`는 현재 세션과
실행 중인 Agent·다른 세션의 목록을 보여주는 경로다. 설정 마법사와 역할이 다르며, 제거된 마법사를 지원 완료 목록으로 올리지 않는다.

복합 Office 표본은 DOCX의 표·이미지·header, XLSX의 수식·병합 셀·chart, PPTX의 표·이미지·chart를 포함한다.
Sol과 Luna의 서로 다른 입력에서 실제 native 도구로 읽기→한 곳 편집→다시 읽기를 검사한다. 별도 검사기는
package part 목록·관계·media와 비대상 XML을 비교하고 파일을 다시 연다. XLSX의 수정 시각 갱신을 별도로 구분하며,
내용·관계 손실을 재직렬화로 간주하지 않는다. 유효한 ZIP 안의 비대상 문단·수식·표·이미지를 바꾼 반례도 거부한다.

Office GUI·매크로·암호 문서·외부 연결·수식 계산 엔진은 이 표본에 포함하지 않는다.
[v0.6.0의 296항목 목록](RELEASE-v0.6.0.md)의 과거 63 PASS/233 NOT_RUN은 유지하며 새 직접 근거를 별도로 연결한다.
남은 식별자를 일괄 PASS 처리하지 않는다. 과거 background timeout의 [원인 미확정](COMPATIBILITY.md#v061-이후-agentbackground-경계-미출하)도 유지한다.
