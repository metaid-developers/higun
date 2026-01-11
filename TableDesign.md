## MempoolSpendDB
key: address_tx:index_timestamp
value:nil
## MempoolIncomeDB
key: address_tx:index_timestamp
value: amount
## utxoStore
key:txId
value:[address@amount@timestamp]
## addressStore
key:address
value:[txid@index@amount@time]
## spendStore
key:address
value:[txid:index@time]