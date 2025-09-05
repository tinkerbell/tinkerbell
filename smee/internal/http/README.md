## Protocol-Based Routing in HTTP Server

This update enhances the `smee/internal/http` package to properly handle protocol-based routing. 
Handlers can now be restricted to serve only HTTP, only HTTPS, or both protocols.

### Features Implemented:

1. **Protocol-Specific Middleware**:
   - Added middleware that validates the incoming request protocol (HTTP or HTTPS) against the handler's allowed protocols
   - Returns 404 Not Found for security reasons when protocols don't match (instead of revealing the endpoint exists)
   - Provides detailed logging when protocol mismatches occur

2. **Protocol Type Enhancement**:
   - Implemented proper `String()` method on the `Protocol` type for better logging and debugging
   - Maintained backward compatibility with existing `protocolString` function

3. **Comprehensive Testing**:
   - Added unit tests for protocol middleware that verify all protocol combinations
   - All existing tests are passing, confirming backward compatibility

4. **Example Application**:
   - Created an example application demonstrating protocol-based routing in action
   - Shows how to register and use handlers for HTTP-only, HTTPS-only, and dual-protocol endpoints

### Usage:

When registering handlers, specify which protocol(s) they should be available on:

```go
handlers := httpserver.HandlerMapping{
    {
        Pattern: "/http-only",
        Handler: httpOnlyHandler,
        Protocols: httpserver.ProtocolHTTP,  // Only accessible via HTTP
    },
    {
        Pattern: "/https-only",
        Handler: httpsOnlyHandler,
        Protocols: httpserver.ProtocolHTTPS, // Only accessible via HTTPS
    },
    {
        Pattern: "/both",
        Handler: bothProtocolsHandler,
        Protocols: httpserver.ProtocolBoth,  // Accessible via both HTTP and HTTPS
    },
}
```

### Security Benefits:

- Ensures sensitive endpoints are only accessible via HTTPS
- Prevents information leakage by not revealing the existence of protocol-restricted endpoints
- Maintains clear separation between public and secure API endpoints

The implementation is thoroughly tested and ready for production use.
