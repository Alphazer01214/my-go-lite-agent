首先进入应用，host发现并挂载了插件。目前挂载了agent, llm-openai, sesison。
agent 是一个组装器，类似deepseek-harness中的agent分为极简模式（仅提供基础的读写工具）、普通模式（提供了全套工具以及沙箱等）
llm-openai 是无状态的chat model api调用器，其输出完全取决于session（或后续需要添加的context-manager）的输入
session 是会话管理，存储了会话的元数据、所有chat message、工具调用、agent turns等信息。包括chat message在内，每一个事件都保存为event，形成一整条trace。后续可以依据此进行对话分支等。此外，未来会引入subagent，因此需要处理parent-child session 的关系。

以上是后端功能层面的。接下来讨论前端设计。

host提供settings，点击弹出各插件的设置（例如llm-openai的base url、api key等）
host提供plugins，点击展示全部发现的插件，以及可视化插件的图依赖关系，同时可对插件进行开关操作

agent通过settings，可以修改其模式
