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

Value: `<container-id>`

#### example

- key: `ip/10.11.0.0/0`
- value: `6dd56e8b-2c74-4c92-bf31-fd3576ed5b03`

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
