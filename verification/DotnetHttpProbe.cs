// Loaded in memory by PowerShell Add-Type. No entry point, installation or global state.
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Net.Sockets;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;

namespace ClauductVerification
{
    public sealed class DotnetHttpProbe
    {
        const string Root = @"C:\Users\JS\.codex";
        const string Cli = @"C:\Users\JS\AppData\Local\Programs\OpenAI\Codex\bin\codex.exe";
        const string Node = @"C:\Program Files\nodejs\node.exe";
        const string Endpoint = "https://chatgpt.com/backend-api/codex/responses";
        const int Limit = 256 * 1024;
        const string Body = "{\"model\":\"gpt-6-astra\",\"instructions\":\"This is a text-only connectivity test. Reply with exactly OK.\",\"input\":[{\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"Reply with exactly OK.\"}]}],\"reasoning\":{\"effort\":\"xhigh\"},\"tools\":[],\"tool_choice\":\"none\",\"stream\":true,\"store\":false}";
        readonly string parser;
        sealed class ProbeFailure : Exception
        {
            public ProbeFailure(string code) : base(code) { }
        }
        static void Require(bool valid, string code) { if (!valid) throw new ProbeFailure(code); }

        public DotnetHttpProbe(string directory) { parser = Path.Combine(directory, "summarize-dotnet-response.mjs"); }

        // Runtimes this probe has been run on and had its observations confirmed. Adding one
        // is a deliberate act, the same as bumping the single pin this replaced.
        static readonly string[] SupportedDotnet = { "10.0.11", "10.0.12" };

        public static void CheckRuntime()
        {
            Require(Array.IndexOf(SupportedDotnet, Environment.Version.ToString()) >= 0
                && OperatingSystem.IsWindows(), "TRANSPORT_RUNTIME_UNSUPPORTED");
            Require(!Debugger.IsAttached, "DEBUG_RUNTIME_UNSUPPORTED");
            // Reject, never clear, runtime injection, trace or TLS overrides. Proxy is disabled per handler.
            foreach (string name in new[] { "NODE_OPTIONS", "NODE_DEBUG", "NODE_USE_ENV_PROXY", "NODE_TLS_REJECT_UNAUTHORIZED",
                "NODE_EXTRA_CA_CERTS", "SSLKEYLOGFILE", "DOTNET_STARTUP_HOOKS", "DOTNET_DiagnosticPorts",
                "DOTNET_EnableEventPipe", "DOTNET_EventPipeOutputPath", "CORECLR_ENABLE_PROFILING", "CORECLR_PROFILER",
                "CORECLR_PROFILER_PATH", "COR_ENABLE_PROFILING", "COR_PROFILER", "COMPlus_EnableEventPipe",
                "DOTNET_DbgEnableMiniDump", "COMPlus_DbgEnableMiniDump" })
                Require(Environment.GetEnvironmentVariable(name) == null, "DEBUG_RUNTIME_UNSUPPORTED");
        }

        static async Task<string> ReadBounded(StreamReader reader, int limit, CancellationToken token)
        {
            var buffer = new char[limit + 1];
            int length = 0;
            while (length <= limit)
            {
                int count = await reader.ReadAsync(buffer.AsMemory(length), token);
                if (count == 0) return new string(buffer, 0, length);
                length += count;
            }
            throw new ProbeFailure("LOCAL_CHECK_FAILED");
        }

        // Fixed executable and ArgumentList only; untrusted bytes use private stdin, never shell syntax.
        // Stderr is bounded and discarded. Child timeout/failure cannot print arbitrary diagnostics.
        static async Task<string> RunProcess(string executable, string[] arguments, string input, CancellationToken cancellation = default)
        {
            var start = new ProcessStartInfo(executable) { UseShellExecute = false, CreateNoWindow = true,
                RedirectStandardInput = true, RedirectStandardOutput = true, RedirectStandardError = true,
                StandardInputEncoding = new UTF8Encoding(false), StandardOutputEncoding = new UTF8Encoding(false, true) };
            foreach (string argument in arguments) start.ArgumentList.Add(argument);
            using var process = new Process { StartInfo = start };
            using var timer = CancellationTokenSource.CreateLinkedTokenSource(cancellation);
            // Generous on purpose: the caller's deadline is the one meant to decide, and at 7000
            // this preempted it. A loopback case came back LOCAL_CHECK_FAILED at 11361ms because
            // the parser spawn outlasted 7s on a runner that stalls process starts, and the mode
            // budget above never got to rule. The linked token still ends this with the caller.
            timer.CancelAfter(60000);
            Require(process.Start(), "LOCAL_CHECK_FAILED");
            try
            {
                Task<string> output = ReadBounded(process.StandardOutput, 16384, timer.Token);
                Task<string> error = ReadBounded(process.StandardError, 4096, timer.Token);
                if (input != null) await process.StandardInput.WriteAsync(input.AsMemory(), timer.Token);
                process.StandardInput.Close();
                await process.WaitForExitAsync(timer.Token);
                string text = await output;
                string diagnostics = await error;
                Require(process.ExitCode == 0 && diagnostics.Length == 0, "LOCAL_CHECK_FAILED");
                return text.Trim();
            }
            finally
            {
                if (!process.HasExited) { process.Kill(true); process.WaitForExit(2000); }
            }
        }

        static string ReadSmall(string path)
        {
            using var file = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
            var bytes = new byte[65537];
            try
            {
                int count = file.ReadAtLeast(bytes, bytes.Length, throwOnEndOfStream: false);
                Require(count <= 65536, "FILE_TOO_LARGE");
                return new UTF8Encoding(false, true).GetString(bytes, 0, count);
            }
            finally { Array.Clear(bytes); }
        }

        static void CheckStore(string config)
        {
            string top = Regex.Split(config, @"^\s*\[", RegexOptions.Multiline)[0];
            string[] lines = top.Split('\n').Where(line => Regex.IsMatch(line, @"^\s*cli_auth_credentials_store\s*=")).ToArray();
            if (lines.Length == 0) return;
            Require(lines.Length == 1, "CONFIG_UNSUPPORTED");
            Match match = Regex.Match(lines[0], "^\\s*cli_auth_credentials_store\\s*=\\s*[\"'](file|keyring|auto)[\"']\\s*(?:#.*)?$");
            Require(match.Success, "CONFIG_UNSUPPORTED");
            Require(match.Groups[1].Value == "file", "CREDENTIAL_STORE_UNSUPPORTED");
        }

        static (string access, string account) SelectCredential(string raw, long now)
        {
            try
            {
                using var doc = JsonDocument.Parse(raw);
                JsonElement root = doc.RootElement;
                Require(!root.TryGetProperty("auth_mode", out var mode) || mode.GetString() == "chatgpt", "INVALID_AUTH_CACHE");
                var tokens = root.GetProperty("tokens");
                string access = tokens.GetProperty("access_token").GetString();
                string account = tokens.GetProperty("account_id").GetString();
                Require(access != null && access.Length <= 16000 && Regex.IsMatch(access, @"\A[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\z")
                    && account != null && Regex.IsMatch(account, @"\A[A-Za-z0-9_-]{1,128}\z"), "INVALID_AUTH_CACHE");
                string payload = access.Split('.')[1].Replace('-', '+').Replace('_', '/');
                payload = payload.PadRight((payload.Length + 3) / 4 * 4, '=');
                using var claims = JsonDocument.Parse(Convert.FromBase64String(payload));
                double expiry = claims.RootElement.GetProperty("exp").GetDouble();
                Require(double.IsFinite(expiry), "INVALID_AUTH_CACHE");
                Require(expiry * 1000 > now + 60000, "TOKEN_EXPIRED");
                // Local expiry sanity only; the upstream verifies token authenticity.
                return (access, account);
            }
            catch (ProbeFailure) { throw; }
            catch { throw new ProbeFailure("INVALID_AUTH_CACHE"); }
        }

        static JsonObject Failure(string category) => new JsonObject { ["passed"] = false, ["category"] = category };
        static string Finish(JsonObject result, int attempts, int connections = 0, bool reconnectBlocked = false)
        {
            result["transport"] = "dotnet-httpclient";
            result["requestedModel"] = "gpt-6-astra";
            result["requestedEffort"] = "xhigh";
            result["clientVersion"] = "0.153.4";
            // Both versions are the ones that ran. The .ps1 gate decides which are allowed and
            // hands its own value over; a literal here would assert a version nothing measured.
            result["powerShellVersion"] = Environment.GetEnvironmentVariable("CLAUDUCT_PWSH_VERSION") ?? "";
            result["dotnetVersion"] = Environment.Version.ToString();
            result["requestAttempts"] = attempts; // SendAsync calls; not proof the server received it.
            result["credentialWrites"] = 0;
            result["retries"] = 0; // No second HTTP send is permitted; blocked reconnects have a separate field.
            result["connectionAttempts"] = connections;
            result["reconnectBlocked"] = reconnectBlocked;
            result["http11Exact"] = true;
            result["proxyEnabled"] = false;
            result["automaticAuthentication"] = false;
            result["automaticDecompression"] = false;
            return result.ToJsonString();
        }

        public async Task<string> Live()
        {
            try
            {
                CheckRuntime();
                string home = Environment.GetEnvironmentVariable("CODEX_HOME");
                Require(string.Equals(Path.GetFullPath(string.IsNullOrEmpty(home) ? Root : home).TrimEnd('\\'), Root,
                    StringComparison.OrdinalIgnoreCase), "UNEXPECTED_CODEX_HOME");
                string version;
                try { version = await RunProcess(Cli, new[] { "--version" }, null); }
                catch { throw new ProbeFailure("CLI_VERSION_CHANGED"); }
                Require(version == "codex-cli 0.153.4", "CLI_VERSION_CHANGED");
                Require(await RunProcess(Node, new[] { "--version" }, null) == "v24.19.0", "TRANSPORT_RUNTIME_UNSUPPORTED");
                (string access, string account) credential;
                try
                {
                    CheckStore(ReadSmall(Path.Combine(Root, "config.toml")));
                    credential = SelectCredential(ReadSmall(Path.Combine(Root, "auth.json")), DateTimeOffset.UtcNow.ToUnixTimeMilliseconds());
                }
                catch (ProbeFailure) { throw; }
                catch { throw new ProbeFailure("FILE_CACHE_UNAVAILABLE"); }
                return await Send(new Uri(Endpoint), credential.access, credential.account, 45000, false);
            }
            catch (ProbeFailure ex) { return Finish(Failure(ex.Message), 0); }
            catch { return Finish(Failure("LOCAL_CHECK_FAILED"), 0); }
        }

        public Task<string> Loopback(int port, string mode)
        {
            Require(port >= 1 && port <= 65535 && new[] { "normal", "short-timeout", "deadline-45s", "cancel-after-headers" }.Contains(mode), "LOCAL_CHECK_FAILED");
            // normal is the mode that must not time out, so its budget is a guard rather than a
            // measurement: short-timeout at 150ms and deadline-45s are what establish that the
            // deadline fires at all. The budget also has to cover the parser process started
            // below, which is a spawn rather than a round trip, and CI has stalled spawns past
            // 30s -- a loopback case came back TIMEOUT at 6411ms against the old 5000.
            return Send(new Uri($"http://127.0.0.1:{port}/probe"), null, null,
                mode == "deadline-45s" ? 45000 : mode == "short-timeout" ? 150 : 30000, mode == "cancel-after-headers");
        }

        async Task<string> Send(Uri uri, string access, string account, int timeoutMs, bool cancelAfterHeaders)
        {
            int connectCalls = 0, connections = 0, attempts = 0;
            bool cancelled = false;
            using var deadline = new CancellationTokenSource(timeoutMs);
            ConsoleCancelEventHandler cancel = (sender, args) => { args.Cancel = true; cancelled = true; deadline.Cancel(); };
            Console.CancelKeyPress += cancel;
            JsonObject result;
            try
            {
                string contentType;
                bool present;
                int status;
                byte[] bytes;
                // Dispose network and authentication-bearing request before parsing untrusted response bytes.
                using (var handler = new SocketsHttpHandler { AllowAutoRedirect = false, UseProxy = false,
                    UseCookies = false, Credentials = null, DefaultProxyCredentials = null, PreAuthenticate = false,
                    AutomaticDecompression = DecompressionMethods.None, MaxConnectionsPerServer = 1,
                    MaxResponseHeadersLength = 16, ActivityHeadersPropagator = null,
                    MaxResponseDrainSize = 0, ResponseDrainTimeout = TimeSpan.Zero })
                {
                    handler.ConnectCallback = async (context, token) =>
                    {
                        Require(Interlocked.Increment(ref connectCalls) == 1, "RECONNECT_REFUSED");
                        Require(context.DnsEndPoint.Host == uri.Host && context.DnsEndPoint.Port == uri.Port, "DESTINATION_REFUSED");
                        var socket = new Socket(SocketType.Stream, ProtocolType.Tcp) { NoDelay = true };
                        try
                        {
                            Interlocked.Increment(ref connections);
                            await socket.ConnectAsync(context.DnsEndPoint, token);
                            return new NetworkStream(socket, ownsSocket: true);
                        }
                        catch { socket.Dispose(); throw; }
                    };
                    // Exact HTTP/1.1 removes HTTP/2/3 stream retries. The callback refuses reconnection.
                    // TLS stays inside HttpClient with its default validation; no certificate callback.
                    using var client = new HttpClient(handler) { Timeout = Timeout.InfiniteTimeSpan };
                    using var request = new HttpRequestMessage(HttpMethod.Post, uri) { Version = HttpVersion.Version11,
                        VersionPolicy = HttpVersionPolicy.RequestVersionExact, Content = new ByteArrayContent(Encoding.UTF8.GetBytes(Body)) };
                    request.Content.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("application/json");
                    request.Headers.Add("Accept", "text/event-stream");
                    request.Headers.Add("Accept-Encoding", "identity");
                    request.Headers.Add("Version", "0.153.4");
                    request.Headers.Add("User-Agent", "codex-cli/0.153.4 (Windows; x64)");
                    request.Headers.Add("originator", "codex_cli_rs");
                    request.Headers.Add("Openai-Beta", "responses=experimental");
                    request.Headers.ConnectionClose = true;
                    request.Headers.ExpectContinue = false;
                    if (access != null)
                    {
                        request.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", access);
                        request.Headers.Add("chatgpt-account-id", account);
                    }
                    attempts++;
                    using var response = await client.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, deadline.Token);
                    status = (int)response.StatusCode;
                    present = response.Content.Headers.NonValidated.TryGetValues("Content-Type", out var values);
                    contentType = present ? string.Join(",", values) : "";
                    Require(contentType.Length <= 16384, "RESPONSE_HEADERS_TOO_LARGE");
                    if (cancelAfterHeaders) { cancelled = true; deadline.Cancel(); }
                    using var stream = await response.Content.ReadAsStreamAsync(deadline.Token);
                    var buffer = new byte[Limit + 1];
                    int length = 0;
                    try
                    {
                        while (true)
                        {
                            int count = await stream.ReadAsync(buffer.AsMemory(length), deadline.Token);
                            if (count == 0) break;
                            length += count;
                            Require(length <= Limit, "RESPONSE_TOO_LARGE");
                        }
                        bytes = buffer.AsSpan(0, length).ToArray();
                    }
                    finally { Array.Clear(buffer); }
                }
                access = null; account = null;
                string envelope = JsonSerializer.Serialize(new { status, contentType, contentTypePresent = present,
                    body = Convert.ToBase64String(bytes) });
                Array.Clear(bytes);
                // No credentials go to the parser. Raw data stays in memory/private stdin, never stdout or files.
                try { result = JsonNode.Parse(await RunProcess(Node, new[] { parser }, envelope, deadline.Token)).AsObject(); }
                catch (OperationCanceledException) when (deadline.IsCancellationRequested) { throw; }
                catch { throw new ProbeFailure("LOCAL_CHECK_FAILED"); }
            }
            catch (ProbeFailure ex) { result = Failure(ex.Message); }
            catch (OperationCanceledException) { result = Failure(cancelled ? "CANCELLED" : "TIMEOUT"); }
            catch (HttpRequestException ex)
            {
                result = Failure(connectCalls > 1 ? "RECONNECT_REFUSED"
                    : ex.HttpRequestError == HttpRequestError.ResponseEnded ? "RESPONSE_TRUNCATED" : "NETWORK_OR_TLS_ERROR");
            }
            catch (IOException) { result = Failure("RESPONSE_TRUNCATED"); }
            catch { result = Failure("LOCAL_CHECK_FAILED"); }
            finally { Console.CancelKeyPress -= cancel; }
            return Finish(result, attempts, connections, connectCalls > 1);
        }

        public string SelfTest()
        {
            int count = 0;
            void Check(bool ok) { Require(ok, "SELF_TEST_FAILED"); count++; }
            void Reject(Action action, string category)
            {
                try { action(); } catch (ProbeFailure ex) { Check(ex.Message == category); return; }
                throw new ProbeFailure("SELF_TEST_FAILED");
            }
            string Fixture(string claims, string account = "synthetic-account") => JsonSerializer.Serialize(new { auth_mode = "chatgpt",
                tokens = new { access_token = "synthetic." + Convert.ToBase64String(Encoding.UTF8.GetBytes(claims)).TrimEnd('=').Replace('+', '-').Replace('/', '_') + ".synthetic", account_id = account } });
            CheckStore(""); Check(true);
            CheckStore("cli_auth_credentials_store = 'file' # test\n"); Check(true);
            Reject(() => CheckStore("cli_auth_credentials_store = 'keyring'"), "CREDENTIAL_STORE_UNSUPPORTED");
            Reject(() => CheckStore("cli_auth_credentials_store = 'auto'"), "CREDENTIAL_STORE_UNSUPPORTED");
            Reject(() => CheckStore("cli_auth_credentials_store = false"), "CONFIG_UNSUPPORTED");
            Reject(() => CheckStore("cli_auth_credentials_store = 'file'\ncli_auth_credentials_store = 'file'"), "CONFIG_UNSUPPORTED");
            Check(SelectCredential(Fixture("{\"exp\":3600}"), 0).account == "synthetic-account");
            Reject(() => SelectCredential(Fixture("{\"exp\":60}"), 0), "TOKEN_EXPIRED");
            Reject(() => SelectCredential(Fixture("{\"exp\":0}"), 0), "TOKEN_EXPIRED");
            foreach (string raw in new[] { "{", "{}", "null", "[]", Fixture("{}"), Fixture("{\"exp\":\"3600\"}"),
                Fixture("{\"exp\":3600}", "bad\r\ninjected"), Fixture("{\"exp\":3600}", "bad\n") })
                Reject(() => SelectCredential(raw, 0), "INVALID_AUTH_CACHE");
            return JsonSerializer.Serialize(new { offlineTests = count, passed = count, credentialReads = 0, externalRequests = 0 });
        }
    }
}
