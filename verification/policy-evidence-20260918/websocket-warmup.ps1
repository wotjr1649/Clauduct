$ErrorActionPreference = 'Stop'
# Called only by the Go evidence test. Headers arrive over stdin, never argv or disk.
# Fixed destination, normal TLS verification, redirects disabled, one request.
$socket = [System.Net.WebSockets.ClientWebSocket]::new()
$handler = [System.Net.Http.SocketsHttpHandler]::new()
$handler.AllowAutoRedirect = $false
$handler.SslOptions.EnabledSslProtocols = [System.Security.Authentication.SslProtocols]::Tls12 -bor [System.Security.Authentication.SslProtocols]::Tls13
$invoker = [System.Net.Http.HttpMessageInvoker]::new($handler)
$deadline = [System.Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(30))
$stage = 'input'
try {
    $inputData = [Console]::In.ReadToEnd() | ConvertFrom-Json -AsHashtable
    foreach ($key in @('Authorization', 'Chatgpt-Account-Id', 'Version', 'User-Agent', 'Originator')) {
        $socket.Options.SetRequestHeader($key, [string]$inputData.headers[$key][0])
    }
    $socket.Options.SetRequestHeader('OpenAI-Beta', 'responses_websockets=2026-02-06')
    $stage = 'connect'
    $null = $socket.ConnectAsync([Uri]'wss://chatgpt.com/backend-api/codex/responses', $invoker, $deadline.Token).GetAwaiter().GetResult()
    $payload = [System.Text.Encoding]::UTF8.GetBytes(($inputData.payload | ConvertTo-Json -Depth 32 -Compress))
    $stage = 'send'
    $null = $socket.SendAsync([ArraySegment[byte]]::new($payload), [System.Net.WebSockets.WebSocketMessageType]::Text, $true, $deadline.Token).GetAwaiter().GetResult()
    $stage = 'receive'
    $buffer = [byte[]]::new(65536)
    for ($i = 0; $i -lt 64; $i++) {
        $message = [System.IO.MemoryStream]::new()
        try {
            do {
                $frame = $socket.ReceiveAsync([ArraySegment[byte]]::new($buffer), $deadline.Token).GetAwaiter().GetResult()
                if ($frame.MessageType -ne [System.Net.WebSockets.WebSocketMessageType]::Text) { throw 'WS_NON_TEXT' }
                if ($message.Length + $frame.Count -gt 1048576) { throw 'WS_TOO_LARGE' }
                $message.Write($buffer, 0, $frame.Count)
            } while (-not $frame.EndOfMessage)
            $eventData = [System.Text.Encoding]::UTF8.GetString($message.ToArray()) | ConvertFrom-Json -AsHashtable
            if ($eventData.type -eq 'response.completed') {
                @{ completed = $true; input_tokens = $eventData.response.usage.input_tokens; output_tokens = $eventData.response.usage.output_tokens; output_items = $eventData.response.output.Count } | ConvertTo-Json -Compress
                exit 0
            }
            if ($eventData.type -in @('error', 'response.failed', 'response.incomplete')) { $stage = 'terminal-failure'; throw 'WS_TERMINAL_FAILURE' }
        } finally { $message.Dispose() }
    }
    throw 'WS_NO_TERMINAL'
} catch {
    @{ completed = $false; stage = $stage } | ConvertTo-Json -Compress
    exit 1
} finally {
    $socket.Abort()
    $socket.Dispose()
    $invoker.Dispose()
    $deadline.Dispose()
}
