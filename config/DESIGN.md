# Dynamic configuration design

DynamicConfig 是配置值及其版本的事实源。版本前置条件阻止过期写入覆盖新值；Watch 将持久化结果传播给消费者。业务协议、分布式锁和外部副作用不属于该模块。
