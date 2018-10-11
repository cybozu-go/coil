`coild` REST API
================

- [GET /status](#status)
- [POST /ip](#post)
- [GET /ip/\<pod-namespace\>/\<pod-name\>](#get)
- [DELETE /ip/\<pod-namespace\>/\<pod-name\>](#delete)

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
  "address-blocks": {
      "default": ["10.20.30.16/28", "10.20.30.48/28"],
      "global": ["1.1.1.0/24"]
  },
  "pods": {
      "default/pod1": "10.20.30.16",
      "another/pod1": "10.20.30.18",
      "global/pod1": "1.1.1.1"
  },
  "status": 200
}
```

## <a name="post" />`POST /ip`

Request a new IP address for the pod.
Input must be a JSON object with these fields:

- `pod-namespace` ... Pod namespace
- `pod-name` ... Pod name
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

- 409 Conflict: when an IP address has been allocated to the pod.
- 503 Service Unavailable: no avaiable IP addresses.
- 500 Internal Server Error: other reasons.

## <a name="get" />`GET /ip/<pod-namespace>/<pod-name>`

Get assigned addresses for the pod.

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

- 404 Not Found: no addresses was assigned to the pod.
- 500 Internal Server Error: other reasons.

## <a name="delete" />`DELETE /ip/<pod-namespace>/<pod-name>`

Release assigned addresses for the pod.

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

- 404 Not Found: no addresses was assigned to the pod.
- 500 Internal Server Error: other reasons.
