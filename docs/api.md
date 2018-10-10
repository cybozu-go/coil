`coild` REST API
================

- [GET /status](#status)
- [POST /ip](#post)
- [GET /ip/\<container-id\>](#get)
- [DELETE /ip/\<container-id\>](#delete)

## Failure response format

Failure response body is a JSON object with these fields:

- `status`: HTTP status code
- `error`: Error message

## <a name="status" />`GET /status`

Obtain `coild` status.

### Successful response

- HTTP status code: 200 OK
- HTTP response header: Content-Type: application/json
- HTTP response body example:

```json
{
  "address-blocks": ["10.20.30.16/28", "10.20.30.48/28"],
  "containers": {
      "container-1": ["10.20.30.16"],
      "container-2": ["10.20.30.18"]
  },
  "status": 200
}
```

## <a name="post" />`POST /`

Request a new IP address for the container.
Input must be a JSON object with these fields:

- `container-id` ... Container ID
- `address-type` (optional) ... `"ipv4"` or `"ipv6"` (default is `"ipv4"`)

### Successful response

- HTTP status code: 200 OK
- HTTP response header: Content-Type: application/json
- HTTP response body: new assigned ip address in JSON
```json
{
  "addresses": ["<ip address>"],
  "status": 200
}
```

### Failure responses

- No avaiable IP addresses: 503 Service Unavailable
- Other error: 500 Internal Server Error

## <a name="get" />`GET /<container-id>`

Get assigned addresses for the container.

### Successful response

- HTTP status code: 200 OK
- HTTP response header: Content-Type: application/json
- HTTP response body: assigned ip address in JSON
```json
{
  "addresses": ["<ip address>"],
  "status": 200
}
```

### Failure responses

- No addresses was assigned to the container: 404 Not Found
- Other error: 500 Internal Server Error

## <a name="delete" />`DELETE /<container-id>`

Release assigned addresses for the container.

### Successful response

- HTTP status code: 200 OK
- HTTP response header: Content-Type: application/json
- HTTP response body: released ip address in JSON
```json
{
  "addresses": ["<ip address>"],
  "status": 200
}
```

### Failure responses

- No addresses was assigned to the container: 404 Not Found
- Other error: 500 Internal Server Error
