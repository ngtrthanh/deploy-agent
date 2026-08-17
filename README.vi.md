# deploy-agent

Deployment reconciler gọn nhẹ cho host Docker Compose.

**English:** [README.md](README.md)

DA được thiết kế theo nguyên tắc **một binary native trên mỗi host**, quản nhiều deployment unit. DA không chạy trong Docker, không cần mở cổng nhận kết nối vào, và production nên chạy dạng one-shot do scheduler của hệ điều hành gọi.

```text
host
└── deploy-agent
    ├── wsm-edge
    ├── matflow
    ├── cems-etl
    └── hpr-traffic
```

Mục tiêu v0.2 rất đơn giản:

```text
Git nói NÊN chạy cái gì
Docker nói ĐANG chạy cái gì
DA làm cho hai bên bằng nhau
```

## Cài đặt

Linux / macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/ngtrthanh/deploy-agent/main/scripts/get.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/ngtrthanh/deploy-agent/main/scripts/get.ps1 | iex
```

Binary có sẵn cho Linux `amd64/arm64/armv7/armv6/386`, Windows `amd64/arm64/386`, macOS `amd64/arm64`. File tải về được kiểm SHA-256.

Kênh `edge` bám theo `main`. Production nên pin một release ổn định `vX.Y.Z`.

## Promotion: điều gì kích hoạt nâng cấp?

Repo source và quyền triển khai là hai việc tách nhau.

| Repo | Trả lời câu hỏi |
|---|---|
| repo ứng dụng | Có những release nào? |
| `deploy-state` | Release nào được phép chạy ở đâu? |

Một feature hoặc bug fix đi theo luồng:

```text
yêu cầu feature / fix bug
    ↓
source PR → CI → merge
    ↓
build image đúng một lần
    ↓
candidate image@sha256:BBBB
    ↓
promotion PR đổi desired digest AAAA → BBBB
    ↓
promotion PR được merge               ← trigger nâng cấp
    ↓
fleet DA polling và tự hội tụ
```

Không cần webhook. Rollback đi đúng con đường đó: promote lại digest đã chạy tốt trước đây.

Ví dụ desired state:

```json
{
  "apiVersion": "deploy/v1",
  "kind": "Release",
  "metadata": {
    "service": "wsm-edge",
    "environment": "edge-prod"
  },
  "spec": {
    "image": "ghcr.io/example/wsm-edge-server",
    "digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "rollout": { "strategy": "recreate" },
    "migration": { "required": false }
  },
  "provenance": {
    "git_sha": "1111111111111111111111111111111111111111"
  }
}
```

`spec.digest` là identity chuẩn để triển khai. Tag chỉ phục vụ đọc và tìm kiếm.

## Kiểm chứng mà không cần sửa app

DA không được ép mọi ứng dụng phải cấy endpoint riêng cho DA.

Artifact identity được chứng minh từ runtime ngay trên host:

```text
desired image@sha256:BBBB
        ↓
docker pull theo digest
        ↓
docker compose recreate
        ↓
docker inspect container đang chạy
        ↓
artifact đang chạy == artifact mong muốn
```

Readiness của ứng dụng là một probe tách riêng. v0.2 nên hỗ trợ:

```text
http          /health, /healthz hoặc URL sẵn có
docker-health Docker HEALTHCHECK
command       lệnh kiểm tra service
tcp           thử kết nối cổng
process       container đang chạy và ổn định
```

Ứng dụng DA-aware có thể bổ sung `/healthz`, `/readyz` và `/api/ops/identity`. Các endpoint này cho bằng chứng ở tầng ứng dụng và observability tốt hơn, nhưng là **tùy chọn**, không phải điều kiện để DA quản ứng dụng.

### Ví dụ WSM Edge: không sửa app

Nếu WSM Edge hiện đã có `GET /health`, DA có thể nâng cấp mà không sửa source:

```text
desired digest BBBB
    ↓
current digest AAAA
    ↓
backup hook
    ↓
pull image@BBBB
    ↓
Compose override pin image@BBBB
    ↓
force-recreate đúng service cần nâng
    ↓
Docker artifact proof
    +
/health hiện có == 200
    ↓
ACCEPT BBBB
```

Nếu kiểm chứng thất bại và deployment đó không chạy migration, DA quay lại digest đã được accept trước đó.

## Reconciliation

Production khuyến nghị:

```text
OS scheduler mỗi 30–60 giây
        ↓
deploy-agent reconcile
        ↓
scan các deployment unit
        ↓
đọc desired → đo actual → hội tụ
        ↓
exit
```

Mô hình này sống qua reboot và crash vì hệ điều hành sẽ gọi lại DA. Nếu nguồn desired state lỗi, DA không được động vào service đang chạy tốt.

Một reconciliation hoàn chỉnh cần có: lock theo deployment unit, timeout cho mọi command/wait, exponential backoff có jitter, giới hạn số lần thử, last-run status bền vững và cache desired state last-known-good.

## Một DA, nhiều deployment unit

Bố cục host mục tiêu:

```text
/etc/deploy-agent/
├── agent.json
└── apps/
    ├── wsm-edge.json
    ├── matflow.json
    └── hpr-traffic.json
```

Windows dùng cùng mô hình dưới `C:\ProgramData\deploy-agent\`.

Lock phải theo deployment unit để một app deploy lâu không khóa các app khác trên cùng host.

## Lệnh

CLI hiện tại:

```bash
deploy-agent -config deploy-agent.json once
deploy-agent -config deploy-agent.json check
deploy-agent -config deploy-agent.json run
deploy-agent -version
```

`once` là kiểu chạy production được khuyến nghị. Fleet/multi-unit orchestration là phần việc v0.2 và chưa được nối vào CLI hiện tại.

## Trạng thái nhánh v0.2

`feature/v0.2-digest-reconciler` là nhánh đang migrate, chưa phải release. Các lớp thấp đang được chuyển từ identity theo Git-SHA/tag sang digest và verification tách lớp. Phần wiring phía trên, fleet mode, generic probes, retry policy, locking, desired-state cache và demo/docs migration vẫn đang làm.

Ở head hiện tại, GitHub CI dừng tại bước `gofmt` trước khi chạy vet/test/build. Không coi nhánh này production-ready cho tới khi CI và Docker E2E demo xanh trở lại.

## Demo

`demo/` hiện vẫn chứng minh end-to-end flow của v0.1. Trước khi v0.2 merge vào `main`, demo phải được chuyển sang `deploy/v1`, deploy pin bằng digest, generic verification, upgrade, rollback và teardown sạch.

## Nguyên tắc thiết kế

- Một DA binary trên mỗi host, không phải một DA cho mỗi container.
- Một deployment unit có thể chứa một hoặc nhiều Compose service.
- Digest là deployment identity; Git SHA là provenance.
- Promotion là thay đổi desired state trong Git, không phải command chạy trên host.
- Artifact proof từ runtime không phụ thuộc việc app tự khai digest của nó.
- App hiện hữu vẫn có thể được DA quản mà không sửa source nếu có probe phù hợp.
- Endpoint DA-aware là bằng chứng mạnh hơn nhưng không bắt buộc.
- Production bình thường chạy one-shot bằng scheduler của hệ điều hành.
- Lỗi khi đọc desired state không được làm ảnh hưởng service đang được accept và chạy tốt.
