# tolo-operation

## 開発

### セットアップ

mise を使用して開発環境をセットアップする  
`mise install` の後処理で lefthook の pre-commit フック（シークレット検出と golangci-lint）が導入される

```bash
mise trust
mise install
cp .env.example .env
task proto
```

`.env` は Compose と `task migrate:*` が開発用 PostgreSQL の接続情報（`POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`）を読む先で、git 管理外

### テストと lint

```bash
go build ./...
go test ./...
golangci-lint run
```

`go test ./...` は統合テストで PostgreSQL コンテナを起動するため、Docker が必要

`task proto:breaking` は、最新のリリースタグの `.proto` と比べて互換性を壊す変更がないかを `buf breaking` で検査する。CI でも同じ検査を行う

### Docker Compose での起動

Docker Compose で PostgreSQL、golang-migrate によるマイグレーション、開発サーバー、依存先のモック（`mockdeps`）を起動できる

```bash
docker compose up --build
```

もしくは
```bash
task up:build
```

サーバーは `http://localhost:8080`、PostgreSQL は `localhost:5432`、`mockdeps` は `http://localhost:8089` で待ち受ける（停止は `task down`）  
Compose の `migrate` サービスは `Dockerfile` の `migrate` ターゲットをビルドして起動するため、`migrations/` を追加・変更したあとは `--build` 付き（`docker compose up --build` または `task up:build`）で起動し直す  
`task up` はイメージを再ビルドしないので、古いマイグレーションのままになる  
Compose は他サービスを起動せず、`mockdeps` が依存先の代わりを務める（`cmd/mockdeps`、開発専用）  
outbox の publish 先のブローカーとして、NATS JetStream の `nats` サービスも起動する（`server` の `NATS_URL` は `nats://nats:4222`）  
`mockdeps` は次の 2 つを 1 つのポートで提供する

- `GET /.well-known/jwks.json`: 起動時に生成した ES256 鍵の JWKS。`server` の `INTERNAL_JWKS_URL` はここを指す
- `POST /token`: 同じ鍵で内部 JWT を発行する（「内部 JWT」を参照）

鍵は `mockdeps` を起動するたびに作り直されるので、再起動の後はトークンを発行し直す

全サービスを実際の RPC でつないだ検証は tolo リポジトリの統合環境で行う

### 環境変数

| 環境変数 | デフォルト | 内容 |
| --- | --- | --- |
| `SERVER_ADDR` | `:8080` | 待ち受けるアドレス |
| `DATABASE_URL` | なし（必須） | PostgreSQL の接続先 |
| `INTERNAL_JWKS_URL` | `http://gateway:8080/.well-known/jwks.json` | 内部 JWT の検証に使う JWKS の取得先（Compose では `mockdeps` の `http://mockdeps:8080/.well-known/jwks.json`） |
| `INTERNAL_JWT_ISSUER` | `service-gateway` | 内部 JWT に期待する `iss` |
| `INTERNAL_JWT_AUDIENCE` | `tolo-operation` | 内部 JWT に期待する `aud` |
| `OUTBOX_DISPATCH_INTERVAL` | `1s` | outbox に積んだメッセージを publish する間隔（Go の duration 形式）。publish 先は `NATS_URL` か `PUBSUB_PROJECT_ID` で選ぶ |
| `NATS_URL` | なし | outbox の publish 先にする NATS JetStream の接続先（Compose では `nats://nats:4222`）。`PUBSUB_PROJECT_ID` とどちらか一方だけを設定する。起動時に `guest-messages` と `realtime-delivery` のストリームを用意する |
| `NATS_STREAM_MAX_AGE` | `24h` | `NATS_URL` を設定したときに、起動時に用意するストリームの保存期限（Go の duration 形式。`1d` は使えないので `24h` と書く）。期限を過ぎたメッセージは配送の済み・未済にかかわらず消える。duration として読めない値か 0 以下の値なら、サーバーは起動しない |
| `PUBSUB_PROJECT_ID` | なし | outbox の publish 先にする Cloud Pub/Sub のプロジェクト。`NATS_URL` とどちらか一方だけを設定する。イベント ID を ordering key にして publish する。どちらも未設定か両方を設定すると、サーバーは起動しない |
| `LOG_LEVEL` | `info` | ログに出力する最小レベル。`debug`／`info`／`warn`（`warning` も同義）／`error`／`critical` を取り、未知の値ならサーバーは起動しない |
| `GOOGLE_CLOUD_PROJECT` | なし | 設定するとログの `logging.googleapis.com/trace` を `projects/<project>/traces/<trace_id>` 形式にし、Cloud Logging でトレースと相関させる。未設定なら素のトレース ID を出力する |
| `OTEL_EXPORTER_OTLP_ENDPOINT` / `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | なし | どちらかを設定したときだけ OTLP/HTTP で span を export する。未設定ならトレーシングは無効 |
| `OTEL_SERVICE_NAME` | `tolo-operation` | トレースの `service.name` |

ヘッダ・TLS・タイムアウトなどその他の `OTEL_EXPORTER_OTLP_*` は exporter がそのまま解釈する  
ログは標準出力へ 1 行 1 件の JSON で書き出す

### マイグレーション

ローカルでマイグレーションを実行する場合は、Compose で PostgreSQL を起動してから次を実行する（接続先は `DATABASE_URL` で上書きできる）  
`task migrate:*` が使う golang-migrate CLI は `.config/mise/conf.d/db.toml` で管理しており、`mise install` で `mise.toml` のツールと一緒に導入される

```bash
# マイグレーションを適用
task migrate:up
# 新しいマイグレーションを作成（up/down のペアを生成）
task migrate:create -- <migration_name>
# 1 つ前にロールバック
task migrate:down
# 現在のバージョンと dirty 状態を表示
task migrate:version
```

### トレースの確認（Jaeger）

監視スタックはオーバーライドファイル `compose.o11y.yml` を重ねたときだけ有効になる  
Jaeger が起動し、`server` に OTLP エクスポート用の環境変数（`OTEL_EXPORTER_OTLP_ENDPOINT` など）がセットされる

```bash
docker compose -f compose.yml -f compose.o11y.yml up --build
```

もしくは
```bash
task up:build:o11y
```

Jaeger UI は `http://localhost:16686`（停止は `task down:o11y`）

### 内部 JWT

RPC を呼び出すには Service Gateway 発行の内部 JWT が必要である  
Compose で起動した場合は、`mockdeps` の `POST /token` でトークンを発行する。`server` は `mockdeps` の JWKS で検証するので、そのまま `Authorization: Bearer <token>` で呼び出せる

```bash
TOKEN=$(curl -s -X POST localhost:8089/token \
  -d '{"token_use":"event_access","tenant_id":"0123456789abcdef","event_id":"fedcba9876543210","subject":"staff-user-1","scope":"events.operate events.read"}' \
  | jq -r .token)

curl -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  localhost:8080/tolo.operation.v1.StaffCommunicationService/SendStaffMessage \
  -d '{"tenant_id":"0123456789abcdef","event_id":"fedcba9876543210","audience":"AUDIENCE_STAFF","body":"hello"}'

curl -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  localhost:8080/tolo.operation.v1.StaffCommunicationService/ListStaffMessages \
  -d '{"tenant_id":"0123456789abcdef","event_id":"fedcba9876543210"}'
```

- 入力の JSON は `audience`（既定 `tolo-operation`）、`token_use`、`tenant_id`、`event_id`、`subject`、`scope`（空白区切り）、`ttl`（Go の duration 形式、既定 `10m`）を取る
- `token_use` は `tenant_access`、`event_access`、`registration`、`service` を取る。本サービスの RPC は `event_access` を要求し、`ListStaffMessages` は `service` も受け付ける。`RequestProposalDelivery` と `RecordFeedbackValues` は `service` だけを受け付ける
- `event_access` では `tenant_id` と `event_id`（どちらも 16 文字 hex）、`scope` が必須である。必要な scope（`events.read`／`events.operate`）は proto の `required_scopes` で宣言されている
- `subject` はトークンの `sub` で、本サービスはこれをメッセージの送信者や接続状態の `user_id` として扱う
- `iss` は `service-gateway` に固定される
- 応答は `token` / `claims` / `jwks` を持つ JSON である

Compose を使わずにトークンを作る場合は、`internal-jwt-handling` 同梱の jwtgen CLI を `go tool jwtgen` で実行する（`go.mod` の `tool` に登録してある）  
出力の `jwks` を任意の HTTP エンドポイントで配信し、`INTERNAL_JWKS_URL` にその URL を設定する

```bash
go tool jwtgen -audience tolo-operation -token-use event_access -tenant-public-id 0123456789abcdef -event-public-id fedcba9876543210 -subject staff-user-1 -scope events.operate -ttl 10m
```

### connect-es の生成

connect-es の生成（`task proto:gen:es`）はリリース時に CI で行う  
ローカルで実行する場合は `clients/connect-es` の依存（`npm i`）を導入する必要がある

## イメージからマイグレーションを実行する

`Dockerfile` の `migrate` ターゲットは golang-migrate CLI のイメージに `migrations/` を `/migrations` として同梱したものである  
リポジトリを clone しなくても、このイメージだけで DB マイグレーションを実行できる  
`ENTRYPOINT` は `migrate -path /migrations` なので、利用者は接続先とコマンドだけを引数として渡す

ローカルでビルドする場合は次のとおりである

```bash
docker build --target migrate -t tolo-operation-migrate .
```

適用は `-database` に接続先を、続けてコマンドを渡す

```bash
docker run --rm --network host tolo-operation-migrate -database "$DATABASE_URL" up
```

公開イメージは `ghcr.io/<owner>/<repo>-migrate` で、サーバーのイメージと同じバージョンタグを付ける

```bash
docker run --rm --network host ghcr.io/<owner>/<repo>-migrate:<version> -database "$DATABASE_URL" up
```

そのほかのコマンドも同じ形で渡す

```bash
# 現在のバージョンを確認
docker run --rm --network host ghcr.io/<owner>/<repo>-migrate:<version> -database "$DATABASE_URL" version
# 1 つ前にロールバック
docker run --rm --network host ghcr.io/<owner>/<repo>-migrate:<version> -database "$DATABASE_URL" down 1
```

golang-migrate CLI は接続先を環境変数からは読まないため、`-database` は必ず引数で渡す  
`--network host` はコンテナからホスト上の PostgreSQL に接続するための指定であり、接続先がホスト外にあるなら不要である

## proto アーティファクトの利用

`.proto` は [ORAS](https://oras.land) で OCI アーティファクト化され、GitHub Container Registry に公開される  
アーティファクト名: `ghcr.io/<owner>/<repo>-proto`

### 取得（pull）

[ORAS CLI](https://oras.land/docs/installation) が必要（mise でも導入される）

```bash
# 出力先ディレクトリに proto を展開（ディレクトリ構造が復元される）
oras pull ghcr.io/<owner>/<repo>-proto:latest -o proto
```

中身は `tolo/operation/v1/operation.proto` だけである  
`operation.proto` は `tolo/kernel/v1/kernel.proto` と `authz/v1/options.proto` を import するので、`buf` や `protoc` の入力にするには、それぞれの成果物も同じディレクトリに展開する

| proto | アーティファクト |
| --- | --- |
| `tolo/kernel/v1/kernel.proto` | `ghcr.io/pj-hoakari/tolo-kernel-proto` |
| `authz/v1/options.proto` | `ghcr.io/pj-hoakari/protoc-gen-authz-go-proto` |

```bash
oras pull ghcr.io/pj-hoakari/tolo-kernel-proto:<version> -o proto
oras pull ghcr.io/pj-hoakari/protoc-gen-authz-go-proto:<version> -o proto
```

版は、このリポジトリの `go.mod` が参照する `github.com/pj-hoakari/tolo-kernel-proto` と `github.com/pj-hoakari/protoc-gen-authz-go` の版以上にする（タグは先頭の `v` を除いた値）  
ほかのサービスの `.proto` と同じディレクトリに展開するときは、各サービスの `go.mod` が参照する版のうち、最も新しいものを使う
