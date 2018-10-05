Etcd Schema
===========

Configurations
----------------

### Address pool object

Key: `pool/<name>`

Value: JSON with these fields

| Name       | Type  | Required | Description                                       |
|:-----------|:------|:---------|:--------------------------------------------------|
| subnets    | array | true     | List of subnets                                   |
| block_size | int   | true     | Size of address block in this pool. Exponent of 2 |

Status
--------

### IP address assignments

Key: `ip/<address-block>/<offset>`

Value: Container ID

Key example: `ip/10.11.0.0/0`

### Address block assignments

Key: `block/<pool>/<subnet-network-address>`

Value:

```json
{
    "free": ["10.11.0.64/27",...],
    "nodes": {
        "node1": ["10.11.0.0/27"],
        "node2": ["10.11.0.32/27"]
    }
}
```
