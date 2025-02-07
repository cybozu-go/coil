# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [pkg/cnirpc/cni.proto](#pkg_cnirpc_cni-proto)
    - [AddResponse](#pkg-cnirpc-AddResponse)
    - [CNIArgs](#pkg-cnirpc-CNIArgs)
    - [CNIArgs.ArgsEntry](#pkg-cnirpc-CNIArgs-ArgsEntry)
    - [CNIArgs.InterfacesEntry](#pkg-cnirpc-CNIArgs-InterfacesEntry)
    - [CNIError](#pkg-cnirpc-CNIError)
  
    - [ErrorCode](#pkg-cnirpc-ErrorCode)
  
    - [CNI](#pkg-cnirpc-CNI)
  
- [Scalar Value Types](#scalar-value-types)



<a name="pkg_cnirpc_cni-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pkg/cnirpc/cni.proto



<a name="pkg-cnirpc-AddResponse"></a>

### AddResponse
AddResponse represents the response for ADD command.

`result` is a types.current.Result serialized into JSON.
https://pkg.go.dev/github.com/containernetworking/cni@v0.8.0/pkg/types/current?tab=doc#Result


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| result | [bytes](#bytes) |  |  |






<a name="pkg-cnirpc-CNIArgs"></a>

### CNIArgs
CNIArgs is a mirror of cni.pkg.skel.CmdArgs struct.
https://pkg.go.dev/github.com/containernetworking/cni@v0.8.0/pkg/skel?tab=doc#CmdArgs


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| container_id | [string](#string) |  |  |
| netns | [string](#string) |  |  |
| ifname | [string](#string) |  |  |
| args | [CNIArgs.ArgsEntry](#pkg-cnirpc-CNIArgs-ArgsEntry) | repeated | Key-Value pairs parsed from CNI_ARGS |
| path | [string](#string) |  |  |
| stdin_data | [bytes](#bytes) |  |  |
| ips | [string](#string) | repeated |  |
| interfaces | [CNIArgs.InterfacesEntry](#pkg-cnirpc-CNIArgs-InterfacesEntry) | repeated |  |






<a name="pkg-cnirpc-CNIArgs-ArgsEntry"></a>

### CNIArgs.ArgsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="pkg-cnirpc-CNIArgs-InterfacesEntry"></a>

### CNIArgs.InterfacesEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [bool](#bool) |  |  |






<a name="pkg-cnirpc-CNIError"></a>

### CNIError
CNIError is a mirror of cin.pkg.types.Error struct.
https://pkg.go.dev/github.com/containernetworking/cni@v0.8.0/pkg/types?tab=doc#Error

This should be added to *grpc.Status by WithDetails()
https://pkg.go.dev/google.golang.org/grpc@v1.31.0/internal/status?tab=doc#Status.WithDetails


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [ErrorCode](#pkg-cnirpc-ErrorCode) |  |  |
| msg | [string](#string) |  |  |
| details | [string](#string) |  |  |





 


<a name="pkg-cnirpc-ErrorCode"></a>

### ErrorCode
ErrorCode enumerates errors for CNIError

| Name | Number | Description |
| ---- | ------ | ----------- |
| UNKNOWN | 0 |  |
| INCOMPATIBLE_CNI_VERSION | 1 |  |
| UNSUPPORTED_FIELD | 2 |  |
| UNKNOWN_CONTAINER | 3 |  |
| INVALID_ENVIRONMENT_VARIABLES | 4 |  |
| IO_FAILURE | 5 |  |
| DECODING_FAILURE | 6 |  |
| INVALID_NETWORK_CONFIG | 7 |  |
| TRY_AGAIN_LATER | 11 |  |
| INTERNAL | 999 |  |


 

 


<a name="pkg-cnirpc-CNI"></a>

### CNI
CNI implements CNI commands over gRPC.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Add | [CNIArgs](#pkg-cnirpc-CNIArgs) | [AddResponse](#pkg-cnirpc-AddResponse) |  |
| Del | [CNIArgs](#pkg-cnirpc-CNIArgs) | [.google.protobuf.Empty](#google-protobuf-Empty) |  |
| Check | [CNIArgs](#pkg-cnirpc-CNIArgs) | [.google.protobuf.Empty](#google-protobuf-Empty) |  |

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

