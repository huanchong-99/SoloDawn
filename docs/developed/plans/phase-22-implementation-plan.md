# Phase 22: WebSocket 事件广播 - TDD 实现计划

> **创建日期:** 2026-02-04
> **工作区:** E:\SoloDawn-phase-22
> **分支:** phase-22-websocket-broadcast

---

## 实施概览

### 架构设计

```
┌─────────────────────────────────────────────────────────────────┐
│                         后端架构                                  │
├─────────────────────────────────────────────────────────────────┤
│  MessageBus (BusMessage)                                        │
│       │                                                         │
│       ▼                                                         │
│  EventBridge (BusMessage → WsEvent)                            │
│       │                                                         │
│       ▼                                                         │
│  SubscriptionHub (per-workflow broadcast channels)              │
│       │                                                         │
│       ▼                                                         │
│  workflow_ws.rs (/ws/workflow/:id/events)                      │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                         前端架构                                  │
├─────────────────────────────────────────────────────────────────┤
│  wsStore.ts (连接管理、心跳、重连、事件分发)                      │
│       │                                                         │
│       ▼                                                         │
│  workflowStore.ts (业务状态累积)                                 │
│       │                                                         │
│       ▼                                                         │
│  UI Components (PipelineView, StatusBar, etc.)                  │
└─────────────────────────────────────────────────────────────────┘
```

### 消息协议

```json
{
  "type": "workflow.status_changed",
  "payload": {
    "workflowId": "uuid",
    "oldStatus": "ready",
    "newStatus": "running"
  },
  "timestamp": "2026-02-04T12:00:00.000Z",
  "id": "evt_abc123"
}
```

### 事件类型映射

| BusMessage | WsEvent Type | Payload |
|------------|--------------|---------|
| StatusUpdate | workflow.status_changed | {workflowId, status} |
| TerminalStatusUpdate | terminal.status_changed | {workflowId, terminalId, status} |
| TaskStatusUpdate | task.status_changed | {workflowId, taskId, status} |
| GitEvent | git.commit_detected | {workflowId, commitHash, branch, message} |
| TerminalCompleted | terminal.completed | {workflowId, terminalId, taskId, status} |
| - | orchestrator.awakened | {workflowId} |
| - | orchestrator.sleeping | {workflowId} |
| - | orchestrator.decision | {workflowId, decision} |
| - | system.heartbeat | {} |

---

## 实施步骤

### Step 1: 后端事件模型定义 (P0)

**文件:** `crates/server/src/routes/workflow_events.rs`

**测试先行:**
```rust
#[test]
fn test_ws_event_serialization() {
    let event = WsEvent::new(
        WsEventType::WorkflowStatusChanged,
        json!({"workflowId": "123", "status": "running"}),
    );
    let json = serde_json::to_string(&event).unwrap();
    assert!(json.contains("workflow.status_changed"));
    assert!(json.contains("timestamp"));
    assert!(json.contains("id"));
}
```

**实现:**
- 定义 `WsEvent` 结构体
- 定义 `WsEventType` 枚举
- 实现序列化

### Step 2: 后端 WebSocket 路由 (P0)

**文件:** `crates/server/src/routes/workflow_ws.rs`

**测试先行:**
```rust
#[tokio::test]
async fn test_workflow_ws_route_exists() {
    // 验证路由注册
}

#[tokio::test]
async fn test_workflow_ws_invalid_id() {
    // 验证无效 workflow_id 返回错误
}
```

**实现:**
- 创建 `/ws/workflow/:id/events` 路由
- 实现 WebSocket 握手
- 实现连接管理

### Step 3: 订阅中心 (P0)

**文件:** `crates/server/src/routes/subscription_hub.rs`

**测试先行:**
```rust
#[tokio::test]
async fn test_subscription_hub_add_remove() {
    let hub = SubscriptionHub::new();
    let rx = hub.subscribe("workflow-1").await;
    assert!(hub.has_subscribers("workflow-1").await);
    drop(rx);
    // 验证清理
}
```

**实现:**
- 实现 per-workflow 广播通道
- 实现订阅/取消订阅
- 实现连接清理

### Step 4: MessageBus 到 WebSocket 桥接 (P1)

**文件:** `crates/server/src/routes/event_bridge.rs`

**测试先行:**
```rust
#[tokio::test]
async fn test_bus_message_to_ws_event() {
    let bus_msg = BusMessage::StatusUpdate {
        workflow_id: "123".to_string(),
        status: "running".to_string(),
    };
    let ws_event = EventBridge::convert(&bus_msg);
    assert_eq!(ws_event.event_type, "workflow.status_changed");
}
```

**实现:**
- 实现 BusMessage → WsEvent 转换
- 实现事件路由到对应 workflow 通道
- 启动后台任务监听 MessageBus

### Step 5: 心跳机制 (P2)

**测试先行:**
```rust
#[tokio::test]
async fn test_heartbeat_sent() {
    // 验证心跳定期发送
}
```

**实现:**
- 服务端每 30 秒发送 `system.heartbeat`
- 客户端更新 lastHeartbeat
- 超时检测与重连

### Step 6: 前端 wsStore 更新 (P3)

**文件:** `frontend/src/stores/wsStore.ts`

**测试先行:**
```typescript
test('should connect to workflow events endpoint', () => {
  // 验证连接到正确的 URL
});

test('should handle workflow.status_changed event', () => {
  // 验证事件处理
});
```

**实现:**
- 添加 `connectToWorkflow(workflowId)` 方法
- 更新事件处理逻辑
- 添加心跳超时检测

### Step 7: 前端组件实时更新 (P3)

**文件:** `frontend/src/components/workflow/PipelineView.tsx`

**测试先行:**
```typescript
test('should update terminal status on ws event', () => {
  // 验证状态更新
});
```

**实现:**
- 订阅 workflow 事件
- 实时更新终端状态
- 实时更新任务状态

### Step 8: StatusBar Orchestrator 状态 (P3)

**文件:** `frontend/src/components/workflow/StatusBar.tsx`

**实现:**
- 显示 Orchestrator 状态 (awakened/sleeping/processing)
- 显示最后心跳时间
- 显示连接状态

---

## 文件清单

### 新增文件
- `crates/server/src/routes/workflow_ws.rs` - WebSocket 路由
- `crates/server/src/routes/workflow_events.rs` - 事件模型
- `crates/server/src/routes/subscription_hub.rs` - 订阅中心
- `crates/server/src/routes/event_bridge.rs` - 事件桥接

### 修改文件
- `crates/server/src/routes/mod.rs` - 注册新路由
- `frontend/src/stores/wsStore.ts` - 更新连接逻辑
- `frontend/src/stores/workflowStore.ts` - 添加事件处理
- `frontend/src/components/workflow/PipelineView.tsx` - 实时更新
- `frontend/src/components/workflow/StatusBar.tsx` - 状态显示

---

## 验收标准

### 功能验收
- [ ] `/ws/workflow/:id/events` 路由可用
- [ ] Workflow 状态变化实时推送
- [ ] Terminal 状态变化实时推送
- [ ] Git 提交事件实时推送
- [ ] Orchestrator 状态可见
- [ ] 心跳机制正常工作
- [ ] 断线自动重连

### 测试验收
- [ ] 后端单元测试通过
- [ ] 前端单元测试通过
- [ ] 集成测试通过
- [ ] Chrome 浏览器全流程测试通过

---

## 执行顺序

1. Step 1-3: 后端基础设施 (事件模型、路由、订阅中心)
2. Step 4: MessageBus 桥接
3. Step 5: 心跳机制
4. Step 6-8: 前端更新
5. 全流程测试
