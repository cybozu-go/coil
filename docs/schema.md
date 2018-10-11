Etcd Schema
===========

Configurations
--------------

### Address pool object

Key: `<prefix>/pool/<name>`

Value: JSON with these fields:

Name       | Type  | Required | Description
---------- | ----- | -------- | -----------
subnets    | array | true     | List of subnets.
block_size | int   | true     | Size of address block in this pool. Exponent of 2.

Status
------

### Existing subnets

Key: `<prefix>/subnet/<subnet-network-address>`

Example: `subnet/10.11.0.0`

### IP address assignments

Key: `<prefix>/ip/<address-block>/<offset>`

Value: `<pod-namespace>/<pod-name>`

#### example

- key: `ip/10.11.0.0/0`
- value: `default/pod-1`

### Address block assignments

Key: `<prefix>/block/<pool>/<subnet-network-address>`

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
