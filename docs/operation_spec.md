# Operation 入出力仕様

package: `tolo.operation.v1`
実現するコンテキスト: operation_context.md（ドメイン定義 operation_domain.md）
役割: スタッフ間連絡、運用操作（配置）、テナント内集約、配送の振り分け（開＝Realtime へ push 委譲、閉＝Notification をトリガー）

手動介入（危険フラグ・スケジュール登録・混雑の手動報告・行列の手動補正）と運用状態の変更（ゲート開閉・観測点設定変更）は誘導への介入であり誘導ドメインに所属するため、本サービスにはない
（Observation の ManualInterventionService 等）

## RPC 一覧

### スタッフ間・ゲスト向けメッセージ

| RPC | 説明（ユビキタス言語） | 呼び出し元 | 認可 | 関連ドメインイベント |
|---|---|---|---|---|
| SendStaffMessage | スタッフメッセージを送る（宛先はスタッフ間またはゲスト向け） | スタッフアプリ | `event_access` + events.operate | StaffMessageSent |
| RevokeStaffMessage | スタッフメッセージを取り下げる（誤送・不要になった案内の撤回） | スタッフアプリ | `event_access` + events.operate | StaffMessageRevoked |
| ShareGuidanceInfo | 誘導方向・誘導制限等、誘導に関する情報を共有する | スタッフアプリ | `event_access` + events.operate | GuidanceInfoShared |
| ListStaffMessages | メッセージを参照する。Guest Service は復旧・突き合わせ時のみ呼ぶ（通常経路は push） | スタッフアプリ、Guest Service（復旧時のみ） | `event_access` + events.read／サービス間（token_use=service） | （参照のみ） |

### 運用操作

| RPC | 説明（ユビキタス言語） | 呼び出し元 | 認可 | 関連ドメインイベント |
|---|---|---|---|---|
| DirectReassignment | 配置替え（担当箇所＝観測点・ゲート・任意のグラフ要素）を指示する | スタッフアプリ | `event_access` + events.operate | ReassignmentDirected |
| ApplyReassignment | 配置変更をシステムに反映する | スタッフアプリ | `event_access` + events.operate | ReassignmentApplied |
| ListReassignments | 配置の現況一覧を返す（管理 UI とスタッフアプリの表示用） | スタッフアプリ、管理 UI | `event_access` + events.read | （参照のみ） |

ゲート開閉（OperateGate）と観測点設定変更（UpdateObservationPointConfig）は観測が直接受ける（Observation）。配置一覧と開閉状態を並べて表示する場合、開閉状態は観測の参照系から取得する

### 配送（観測からの依頼を受ける）

| RPC | 説明（ユビキタス言語） | 呼び出し元 | 認可 | 関連ドメインイベント |
|---|---|---|---|---|
| RequestProposalDelivery | 誘導提案・行列案内の配信依頼を受け、接続状態で配送を振り分ける（開＝`realtime-delivery` トピックへ publish、閉＝Notification.SendPush）。宛先はスタッフのみ | 観測 | サービス間（token_use=service） | DeliveryRouted（委譲先で RealtimeDelivered／PushNotificationSent・Failed） |
| RecordFeedbackValues | Flow が出力したフィードバック値を受け取りテナント内で蓄積する | 観測 | サービス間（token_use=service） | FeedbackAccumulated |
| UpdateConnectionState | スタッフアプリの接続状態（開／閉）の変化を受け取る。配送分岐の根拠 | スタッフアプリ（フォアグラウンド／バックグラウンド遷移時に直接通知） | `event_access` + events.read | AppConnected／AppDisconnected |

### テナント内集約

| RPC | 説明（ユビキタス言語） | 呼び出し元 | 認可 | 関連ドメインイベント |
|---|---|---|---|---|
| GetHistoryDigest | 履歴ダイジェスト（テナント内集約）を参照する | スタッフアプリ | `event_access` + events.read | HistoryDigestAggregated（集約自体は内部バッチ） |

フィードバック値の匿名化（FeedbackAnonymized）と参照値集約への投入は内部バッチで行う
（呼び出す側: Reference Aggregation の SubmitAnonymizedFeedback。境界外へ出すデータは事前匿名化）
匿名化では、テナント識別子を除き、値を上下のパーセンタイルで丸め込む（外れ値のクリップ）。テナント横断の中央値集約と K-匿名性の判定は Reference Aggregation が担う

## 参考 proto 定義

```proto
syntax = "proto3";
package tolo.operation.v1;
import "google/protobuf/timestamp.proto";
import "tolo/kernel/v1/kernel.proto";

service StaffCommunicationService {
  rpc SendStaffMessage(SendStaffMessageRequest) returns (StaffMessage);
  rpc RevokeStaffMessage(RevokeStaffMessageRequest) returns (RevokeStaffMessageResponse);
  rpc ShareGuidanceInfo(ShareGuidanceInfoRequest) returns (StaffMessage);
  rpc ListStaffMessages(ListStaffMessagesRequest) returns (ListStaffMessagesResponse);
}

service OperationControlService {
  rpc DirectReassignment(DirectReassignmentRequest) returns (Reassignment);
  rpc ApplyReassignment(ApplyReassignmentRequest) returns (Reassignment);
  rpc ListReassignments(ListReassignmentsRequest) returns (ListReassignmentsResponse);
}

service DeliveryCoordinationService {
  rpc RequestProposalDelivery(RequestProposalDeliveryRequest) returns (RequestProposalDeliveryResponse);
  rpc RecordFeedbackValues(RecordFeedbackValuesRequest) returns (RecordFeedbackValuesResponse);
  rpc UpdateConnectionState(UpdateConnectionStateRequest) returns (UpdateConnectionStateResponse);
}

service DigestService {
  rpc GetHistoryDigest(GetHistoryDigestRequest) returns (HistoryDigest);
}

// スタッフメッセージ: スタッフ間・ゲスト向けメッセージ
message StaffMessage {
  string message_id = 1;
  string event_id = 2;
  string sender_user_id = 3;
  Audience audience = 4;
  string body = 5;
  google.protobuf.Timestamp sent_at = 6;
}
enum Audience {
  AUDIENCE_UNSPECIFIED = 0;
  AUDIENCE_STAFF = 1;
  AUDIENCE_GUEST = 2;
  AUDIENCE_BOTH = 3;
}

message SendStaffMessageRequest {
  string event_id = 1;
  Audience audience = 2;
  string body = 3;
  string tenant_id = 4;  // 必須。event_id の属するテナント
}
message RevokeStaffMessageRequest {
  string event_id = 1;
  string message_id = 2;
  string tenant_id = 3;  // 必須。event_id の属するテナント
}
message RevokeStaffMessageResponse {}

message ShareGuidanceInfoRequest {
  string event_id = 1;
  string body = 2;
  repeated string related_route_ids = 3;  // 共有する誘導の対象
  string tenant_id = 4;  // 必須。event_id の属するテナント
}
message ListStaffMessagesRequest {
  string event_id = 1;
  Audience audience = 2;  // Guest Service は GUEST のみ許可（復旧pull時）
  string tenant_id = 3;   // 必須。event_id の属するテナント
}
message ListStaffMessagesResponse {
  repeated StaffMessage messages = 1;
}

// 配置／配置替え: スタッフの担当箇所（観測点・ゲート・任意のグラフ要素）の割当と変更
// 同一の配置先に複数のスタッフを置ける。ルート途中は anchor.route_position（表示用途のみ）
message Reassignment {
  string reassignment_id = 1;
  string event_id = 2;
  string staff_user_id = 3;
  oneof target {
    string observation_point_id = 4;
    string gate_point_id = 5;
    tolo.kernel.v1.GraphAnchor anchor = 7;  // 任意のグラフ地点・通路（警備・案内立ち等）
  }
  ReassignmentStatus status = 6;
}
enum ReassignmentStatus {
  REASSIGNMENT_STATUS_UNSPECIFIED = 0;
  REASSIGNMENT_STATUS_DIRECTED = 1;  // 指示済み
  REASSIGNMENT_STATUS_APPLIED = 2;   // システム反映済み
}

message DirectReassignmentRequest {
  string event_id = 1;
  string staff_user_id = 2;
  oneof target {
    string observation_point_id = 3;
    string gate_point_id = 4;
    tolo.kernel.v1.GraphAnchor anchor = 5;
  }
  string tenant_id = 6;  // 必須。event_id の属するテナント
}
message ApplyReassignmentRequest {
  string reassignment_id = 1;
  string event_id = 2;
  string tenant_id = 3;  // 必須。event_id の属するテナント
}
message ListReassignmentsRequest {
  string event_id = 1;
  string tenant_id = 2;  // 必須。event_id の属するテナント
}
message ListReassignmentsResponse {
  repeated Reassignment reassignments = 1;
}

// 提案配信: 提案・行列案内をスタッフ（アプリ）へ配る（宛先はスタッフのみ）
// 内容は本サービスの型で受け取る。観測が Flow／Line の応答から変換する
message RequestProposalDeliveryRequest {
  string event_id = 1;
  oneof payload {
    FlowProposals flow_proposals = 2;
    LineGuidance line_guidance = 3;
  }
  string tenant_id = 4;  // 必須。event_id の属するテナント
}
message RequestProposalDeliveryResponse {
  string delivery_id = 1;        // realtime-delivery へ publish した DeliveryOrder の ID（本サービスが UUIDv7 で採番）
  int32 realtime_count = 2;      // realtime-delivery へ publish した宛先数
  int32 notification_count = 3;  // 閉デバイスへプッシュ依頼した数
}

message RecordFeedbackValuesRequest {
  string event_id = 1;
  FeedbackValues values = 2;
  string tenant_id = 3;  // 必須。event_id の属するテナント
}
message RecordFeedbackValuesResponse {}

// ---- トピックのペイロード（ADR-0106。購読する側は本サービスの Go モジュールを import する） ----

// realtime-delivery トピックのペイロード（購読: Realtime）
message DeliveryOrder {
  string delivery_id = 1;  // 本サービスが採番する UUIDv7（時系列順序。重複排除と resume の基準）
  string event_id = 2;
  repeated string recipient_user_ids = 3;  // 接続状態が開のスタッフ
  DeliveryPayload payload = 4;
}
// 配信内容（Realtime の FetchDeliveries もこの型で返す）
message DeliveryPayload {
  oneof kind {
    FlowProposals flow_proposals = 1;  // 大域誘導の提案
    LineGuidance line_guidance = 2;    // 局所行列誘導の案内・提案
    StaffMessage staff_message = 3;    // スタッフメッセージ・誘導情報共有
  }
}

// guest-messages トピックのペイロード（購読: Guest Service）
message GuestMessagesUpdate {
  string event_id = 1;
  repeated GuestMessage upserts = 2;
  repeated string revoked_message_ids = 3;  // 取り下げ
}
message GuestMessage {
  string message_id = 1;
  string body = 2;
  google.protobuf.Timestamp sent_at = 3;
}

// ---- 大域誘導の提案（Flow の最適化結果を観測が変換） ----
// 対象はグラフ要素の識別子（point_id／route_id）で表す。ルートの向きは from→to を順方向とする

message FlowProposals {
  repeated RouteImportance route_importances = 1;            // 重要度付きルート
  repeated DirectionProposal direction_proposals = 2;        // 方向属性提案
  repeated RestrictionProposal restriction_proposals = 3;    // 通行制限提案
  repeated DetourProposal detour_proposals = 4;              // 迂回ルート
  repeated BoundaryControlProposal boundary_controls = 5;    // 境界制御提案
  google.protobuf.Timestamp solved_at = 6;
}
enum RouteDirection {
  ROUTE_DIRECTION_UNSPECIFIED = 0;
  ROUTE_DIRECTION_FORWARD = 1;     // from→to
  ROUTE_DIRECTION_BACKWARD = 2;    // to→from
  ROUTE_DIRECTION_BOTH_WAYS = 3;   // 両通行
}
message RouteImportance {
  string route_id = 1;
  optional RouteDirection direction = 2;  // 未設定は向きを問わない
  double importance = 3;
}
message DirectionProposal {
  string route_id = 1;
  RouteDirection proposed_direction = 2;
  DirectionChange change = 3;
  double confidence = 4;
}
enum DirectionChange {
  DIRECTION_CHANGE_UNSPECIFIED = 0;
  DIRECTION_CHANGE_CONVERT_ONE_WAY = 1;  // 一方通行化
  DIRECTION_CHANGE_RELEASE_ONE_WAY = 2;  // 一方通行の解除
  DIRECTION_CHANGE_FLIP_ONE_WAY = 3;     // 一方通行の反転
  DIRECTION_CHANGE_KEEP = 4;             // 維持
}
message RestrictionProposal {
  string route_id = 1;
  RestrictionAction action = 2;
  optional double limit_value = 3;  // action=LIMIT のときの上限（人/分）
  RestrictionReason reason = 4;
  double confidence = 5;
}
enum RestrictionAction {
  RESTRICTION_ACTION_UNSPECIFIED = 0;
  RESTRICTION_ACTION_CLOSE = 1;   // 通行止め
  RESTRICTION_ACTION_LIMIT = 2;   // 流入制限
  RESTRICTION_ACTION_RESUME = 3;  // 解除
}
enum RestrictionReason {
  RESTRICTION_REASON_UNSPECIFIED = 0;
  RESTRICTION_REASON_UNDRAINABLE_STAGNATION = 1;  // 排出できない停滞
  RESTRICTION_REASON_RESIDUAL_STAGNATION = 2;     // 最適化後も残る停滞
  RESTRICTION_REASON_PUNCTURE = 3;                // パンク
  RESTRICTION_REASON_DANGER_UPSTREAM = 4;         // 危険箇所の上流
  RESTRICTION_REASON_OVERLOAD = 5;                // 過負荷
}
message DetourProposal {
  string origin_route_id = 1;
  repeated string route_ids = 2;  // 迂回経路（順序つき）
  double confidence = 3;
}
message BoundaryControlProposal {
  string point_id = 1;  // 入退出点
  BoundaryAction action = 2;
}
enum BoundaryAction {
  BOUNDARY_ACTION_UNSPECIFIED = 0;
  BOUNDARY_ACTION_PAUSE_INGRESS = 1;  // 入場の一時停止
  BOUNDARY_ACTION_PAUSE_EGRESS = 2;   // 退場の一時停止
  BOUNDARY_ACTION_RESUME = 3;         // 再開
}

// ---- 局所行列誘導の案内・提案（Line の応答を観測が変換） ----
// ゲスト向け案内ダイジェストは含めない（ゲストへは観測が guest-status で供給する）

message LineGuidance {
  repeated QueueSummary queues = 1;                // 行列の現況
  repeated QueueChange changes = 2;                // 検知した変化
  repeated GateRecommendation recommendations = 3; // 並び先案内（集約案内のみ）
  repeated GateAvailability availability = 4;      // ゲート空き
  repeated QueueShapeProposal shape_proposals = 5; // 行列形状最適化提案
}
message QueueSummary {
  string queue_id = 1;
  repeated string gate_point_ids = 2;
  double queue_length_people = 3;
  optional double queue_length_meters = 4;
  double wait_time_minutes = 5;
  int32 lane_count = 6;
  int32 turnaround_count = 7;
}
message QueueChange {
  string queue_id = 1;
  QueueChangeKind kind = 2;
}
enum QueueChangeKind {
  QUEUE_CHANGE_KIND_UNSPECIFIED = 0;
  QUEUE_CHANGE_KIND_FORMED = 1;             // 行列形成
  QUEUE_CHANGE_KIND_LENGTH_CHANGED = 2;     // 行列長変化（解消を含む）
  QUEUE_CHANGE_KIND_WAIT_TIME_UPDATED = 3;  // 待ち時間更新
  QUEUE_CHANGE_KIND_TURNAROUND_FORMED = 4;  // 折り返し発生
}
message GateRecommendation {
  string gate_point_id = 1;
  string queue_id = 2;
  double expected_wait_minutes = 3;
}
message GateAvailability {
  string gate_point_id = 1;
  bool available = 2;
}
message QueueShapeProposal {
  string queue_id = 1;
  repeated LaneChange lane_changes = 2;
  int32 proposed_turnaround_count = 3;
}
message LaneChange {
  LaneAction action = 1;
  int32 count = 2;
}
enum LaneAction {
  LANE_ACTION_UNSPECIFIED = 0;
  LANE_ACTION_OPEN = 1;   // レーン増設
  LANE_ACTION_CLOSE = 2;  // レーン閉鎖
}

// ---- フィードバック値（Flow の出力を観測が変換） ----
// テナント内蓄積と、匿名化済みフィードバック（Reference Aggregation の AnonymizedFeedback）の材料

message FeedbackValues {
  repeated FeedbackEntry entries = 1;
  google.protobuf.Timestamp computed_at = 2;
}
message FeedbackEntry {
  string attribute_tag = 1;  // 属性タグ。空はイベント全体の値（目的値など）
  string kind = 2;           // 値の種類。匿名化後の AnonymizedFeedback.kind と同じ語彙
  double value = 3;
}

// 接続状態: アプリ開／閉（配送分岐の根拠）
message UpdateConnectionStateRequest {
  string user_id = 1;
  string device_id = 2;
  bool connected = 3;
  google.protobuf.Timestamp at = 4;
  string event_id = 5;
  string tenant_id = 6;  // 必須。event_id の属するテナント
}
message UpdateConnectionStateResponse {}

message GetHistoryDigestRequest {
  string event_id = 1;
  google.protobuf.Timestamp from = 2;
  google.protobuf.Timestamp to = 3;
  string tenant_id = 4;  // 必須。event_id の属するテナント
}
// 履歴ダイジェスト: 最小の構造化。種別の追加は kind の拡張で吸収する
message HistoryDigest {
  string event_id = 1;
  google.protobuf.Timestamp from = 2;
  google.protobuf.Timestamp to = 3;
  repeated DigestEntry entries = 4;
}
message DigestEntry {
  string kind = 1;  // 例: crowd_peak／queue_wait_avg／proposals_issued／feedback_avg
  oneof target {
    string point_id = 2;
    string route_id = 3;
    string queue_id = 4;
  }
  double value = 5;
}
```

## 補足

- 配送の支援機構（Realtime／Notification）は独立コンテキストにしない
  Realtime へは PubSub（`realtime-delivery` トピックへ `DeliveryOrder` を publish。Realtime）、Notification へは SendPush（同期 RPC。Notification）
- audience が GUEST を含むスタッフメッセージは送信時に `guest-messages` トピックへ publish する（`GuestMessagesUpdate`。Guest Service が購読。Guest Service）
  Guest Service は自 DB に保持し、ゲストアクセス時に本サービスへ問い合わせない
- 取り下げ（RevokeStaffMessage）は、audience が GUEST を含む場合に `revoked_message_ids` として同トピックへ publish する
  ListStaffMessages は取り下げ済みメッセージを返さない。復旧 pull の突き合わせでは、返却されるのは有効メッセージの全量であり、受信側は返却に含まれない保持中の message_id を取り下げ扱いにする
- 全 RPC は対象のテナントとイベントをリクエストの `tenant_id` と `event_id` で受け取る（ADR-0040）
  `tenant_id` か `event_id` が空の要求は `invalid_argument` を返す。クレームに `tenant_id`／`event_id` があればリクエストと突合し、不一致は `permission_denied` を返す
  ApplyReassignment は、`reassignment_id` の配置替えが `event_id` のイベントのものでなければ `not_found` を返す
  UpdateConnectionState は、`user_id` がクレームの `sub` と一致しなければ `permission_denied` を返す
- マシン起点の `token_use=service` で呼ばれる ListStaffMessages（Guest Service の復旧 pull）、RecordFeedbackValues と RequestProposalDelivery（観測サイクル）はクレームを持たず、突合できない。3 つのメソッドはこの場合に境界の強制を要さないメソッドとし（service_gateway.md の「サービス側の検証規約」、ADR-0102、ADR-0103）、`event_id` が `tenant_id` に属するかを確かめずに、`tenant_id` と `event_id` の組で区切った区画だけを読み書きする
  組を誤った RequestProposalDelivery は、宛先のスタッフが見つからず、どこにも配送しない
- スタッフメッセージの本文は前後の空白を除いて 1 文字以上 2000 文字以下とする（文字数は Unicode のコードポイントで数える）。満たさない要求は `invalid_argument` を返す
- 取り下げ（RevokeStaffMessage）は、`events.operate` を持つスタッフなら送信者でなくてもできる。誤送や不要になった案内を、送信者が不在でも撤回できるようにするためである
- ListStaffMessages の audience による絞り込みは次のとおりとする。並びは送信時刻の古い順
  STAFF はスタッフ宛てと両方宛て、GUEST はゲスト宛てと両方宛て、BOTH は両方宛てだけ、UNSPECIFIED はすべてを返す
- ListReassignments は、スタッフごとに指示時刻が最も新しい配置替えを 1 件ずつ返す（配置の現況）
- publish（`guest-messages`・`realtime-delivery`）にはイベント単位の順序キーを付与する（トピック規約。spec-base/README.md）
- 提案配信の宛先はスタッフのみ。ゲストへの誘導情報は、提案を受けたスタッフがゲスト向けメッセージとして手動配信する（システムによる自動転送はしない）
- テナント内集約は保護境界内に閉じる。境界外（参照値集約）へ出すのは匿名化済みフィードバックのみ
- 提案・行列案内・フィードバック値の型は本サービスが定義し、Flow Control／Line Control の proto（`tolo.flow.v1`／`tolo.line.v1`）を import しない
  観測が腐敗防止層として Flow／Line の応答から本サービスの型へ変換する（Observation）。Flow／Line が型を変えても、観測の変換を直すまで本サービスの型は変わらない
  フィードバック値の種類（`FeedbackEntry.kind`）の語彙と、どの値を載せるかは実装フェーズで確定する
- ゲート開閉・観測点設定変更は誘導への介入として観測が直接受ける（Observation の OperateGate／UpdateObservationPointConfig）。本サービスから観測への同期 RPC は存在しない
- ゲスト向け表示の文言化・表示名の付与は Guest Service が担う（Guest Service）
  本サービスがゲストへ push するスタッフメッセージは、スタッフが書いた本文のままで文言化は行わない
- 配置は同一の配置先（グラフ要素・観測点・ゲート）に複数のスタッフを置ける
  ルート途中の位置は `GraphAnchor.route_position`（比率）で表し、表示用途のみに使う
- 蓄積型の供給（`guest-messages`・`realtime-delivery` への publish）は PubSub を経由する
  配信の再試行はブローカーに委譲（at-least-once）し、送信側の再試行は publish 失敗時のみ（超過はログ・アラート）
  publish は transactional outbox から Watermill の publisher で行う。ブローカーは環境ごとに Cloud Pub/Sub と NATS JetStream を使い分ける（ADR-0107）
  ペイロード（`DeliveryOrder`・`GuestMessagesUpdate`）の型は本サービスの package が定義する（ADR-0106）
