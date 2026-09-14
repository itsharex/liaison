# Ollama 原生上游

在应用的模型接口配置中选择 **Ollama**，API 路径默认 `/api`。
保存后检测服务，通过 `GET /api/tags` 获取模型名称，再建立 OpenAI 访问并配置公开模型别名。

当前链路：OpenAI 客户端 → Liaison 密钥/额度校验 → 连接器 → Ollama `/api/chat`。
对外接口仍为 `/v1/chat/completions`，不是 Ollama 原生客户端入口。

支持文本 system/user/assistant 消息、非流式 JSON 和 NDJSON → SSE 流式转换；
temperature、top_p、seed、stop，以及 max_tokens → options.num_predict。
响应中的 thinking 单独映射为 reasoning_content，不混入回答正文。
结束帧的 prompt_eval_count / eval_count 进入现有 Token 计量；缺失计量不视为零。
流中断或无 done:true 结束帧视为失败，不生成成功的 [DONE]。

首版不支持图片、工具调用、任意原生 options、keep_alive、模型拉取/删除/创建接口。
不支持的请求字段返回错误，不静默丢弃。所有上游流量仍经过授权连接器，
不转发客户端的认证头，不暴露上游模型名称或错误正文。

接口依据：[Chat](https://docs.ollama.com/api/chat)、
[Tags](https://docs.ollama.com/api/tags)、[Streaming](https://docs.ollama.com/api/streaming)。
