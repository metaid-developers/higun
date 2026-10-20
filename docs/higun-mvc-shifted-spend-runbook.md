# higun MVC 错归属花费（shifted spend）事故 runbook

- 日期：2026-10-20
- 影响链：assist-base-service 补贴池补货依赖 higun `/utxos`；三个充值池地址 `/balance` 与 `/utxos` 数值分裂
- 修复提交：9cff490（代码）、a910fd4（修复 CLI）
- 上游结论先行：**`/balance` 是错的一侧，`/utxos` 是对的一侧**。1LFYPB 真实未花仅 1 个 UTXO / 175,808 sat（节点 gettxout 验证），`/balance` 报的 19 个 / 11,068,196,528 sat 中 18 个是幻影。

## 根因（两层）

1. **utxoStore 行被 mempool 写入污染**：`mempool.processOutputs` 曾对 ZMQ 看到的交易直接 `Set` 主键行（`txid`），内容为 3 字段条目（`address@amount@time`，无 vout）。与确认索引器 Merge 写入的 4 字段条目（`address@amount@time@vout`）混在同一行，形成 `[部分 3 字段前缀][完整 4 字段]` 错位行；个别行（如 921d0141）被 Set 整体覆盖只剩 2 条。触发条件未完全定位（ZMQ 重复投递/部分行写），修复后该路径写 `mem:` 前缀键，不再可能发生。
2. **按位置取地址**：`processSpend` 经 `QueryUTXOAddresses/QueryUTXODetails` 解析输入归属时按「第 N 个逗号段」取地址。错位行上 vout≥2 的解析整体偏移：gas 池 1M1AY 的 `:1` 花费被记到 1LFYPB（10 条），1LFYPB 拆分找零 `:2000` 的真实花费（17 条）被记到 1M1AY，另有 `88c9e085:950` 的花费因当时行只剩 2 条而**被丢弃**（spender=f9509728，blocktime 1780901841，已用 fix_single_spend 补录）。

后果：1LFYPB 的 spend 行缺 18 条真实花费 → income−spend=19 幻影未花 → BalanceIndex 一致地错 → `/balance` 虚高 11,068,196,528 sat。`/utxos` 因 2026-05 引入的节点 gettxout 校验器（`UTXOValidationEnabled`）把幻影过滤掉反而返回了真值 count=1。这也解释了 2026-09-02 事故后「/balance 与 /utxos 求和一致」验收为何在本次失效：两路数据本身都源自同一组被污染的 store，只是 /utxos 多了一层链上校验。

## 代码修复（9cff490）

- 存储层解析按显式 vout 字段匹配；纯无 vout 行（纯 mempool 行）才按位置兜底（`storage.resolveUtxoRowEntry`，导出 `storage.ExtractAddressFromUtxoRow`）。
- mempool 写行改 `mem:` 前缀键，`GetUtxoAddress` 主键 miss 后回退查 `mem:`。
- `indexIncome` 写 utxoStore 由 Merge 改为整行 Set（`BulkSetMapConcurrent`），行是完整快照，重放幂等，重处理可自愈污染行。
- `/utxo/check` 端点同一解析器。

## 数据修复（a910fd4 工具）

前置：先部署新二进制（防新增脏数据），再修数据。

```bash
# 1) 停容器（工具直接打开 Pebble 目录，需独占文件锁）
sudo docker stop higun_new

# 2) 演练（对 /mnt2/sandbox 的分片拷贝先跑 scan+apply，确认计划与结果）
/tmp/repair_shifted_spends -data-dir /mnt2/sandbox -shards 4 \
  -addresses 1LFYPB16N2h1Xrbdqjbz285ANqHWcWsLRu,1M1AYMf9GqrNGjzJpCqNDP2hvj6ywPsw1G,1Md4GKJ1KGhQs79jcTEqwLtjWoXmphbSXf,18HscT1cqwhjzyD95TjXtphzj2sQJAv1bv

# 3) 生产执行：迁移错归属记录 + 清理污染行 + 清 tracked 余额行
/tmp/repair_shifted_spends -data-dir /mnt2/higun_from_8_217/data -shards 4 -apply -clear-balance \
  -addresses 1LFYPB16N2h1Xrbdqjbz285ANqHWcWsLRu,1M1AYMf9GqrNGjzJpCqNDP2hvj6ywPsw1G,1Md4GKJ1KGhQs79jcTEqwLtjWoXmphbSXf,18HscT1cqwhjzyD95TjXtphzj2sQJAv1bv

# 4) 补录当年被丢弃的花费记录
/tmp/fix_single_spend -data-dir /mnt2/higun_from_8_217/data \
  -address 1LFYPB16N2h1Xrbdqjbz285ANqHWcWsLRu \
  -record 88c9e08537dbee0a0c124a19ba1f78f722d73d3120f73ecb9bc1fb1d48bb1536:950@1780901841@f9509728f167d2c65c82afba3254c91fdf7b5ca72c8644a33ea0b22a5d5a3a37

# 5) 起容器并验收
sudo docker start higun_new
```

## 验收

- `GET /balance?address=1LFYPB…` 的 confirmed_utxo_count / confirmed_balance_satoshi 与 `GET /utxos` 条数、求和一致，且等于节点真值（gettxout 可见的未花集）：1LFYPB = 1 / 175,808。
- 同样核对 1M1AY、1Md4GK、18HscT。
- 回归：13ZLeYbqc4MT9HZGQRiamf93cNANZwR2Ec 的 `/utxos` 与 `/balance` 总额保持修复前观测值；`sudo docker logs --since 10m higun_new 2>&1 | grep -c panic:` 为 0。
- 抽查 `/utxo/db?tx=9d7ef0d3…`：行内应只剩带 vout 的 4 字段条目。

## 业务影响（必须知晓）

11,068,196,528 sat 的「库存」是索引幻影：这些输出早已被池子自身的拆分/补充 gas 链路消费（例如 ff797203:2000 2.99 亿 sat 已由 a066dd0b 消费并拆成 2000 个 100,300 sat 的 gas 块，链上 18,103 个确认）。修复后 assist 的 Job-Topup-auto 会看到真实库存（1LFYPB 仅 175,808 sat，不足单笔 500,000 补贴），补货仍会失败——**reward 池需要业务侧重新注资**，这不是索引层能解决的。

## 后续建议

- 其它地址可能残留同样的错归属记录（本工具按需扩 `-addresses` 即可增量修复；全量扫描可仿照 tools/scan_missing_spends 做 shard 遍历版）。
- 监控点：`[UTXO] WARNING: N unresolved dbQueryPoints` 每日增量应趋近于 0；不再增长说明归属链路已闭合。
