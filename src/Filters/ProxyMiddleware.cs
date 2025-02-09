#nullable enable
using System.Text.RegularExpressions;
using LocalTest.Configuration;
using Microsoft.Extensions.Options;

namespace LocalTest.Filters;

public class ProxyMiddleware
{
    private readonly RequestDelegate _nextMiddleware;
    private readonly IOptions<LocalPlatformSettings> localPlatformSettings;
    private readonly ILogger<ProxyMiddleware> _logger;

    public ProxyMiddleware(
        RequestDelegate nextMiddleware,
        IOptions<LocalPlatformSettings> localPlatformSettings,
        ILogger<ProxyMiddleware> logger
    )
    {
        _nextMiddleware = nextMiddleware;
        this.localPlatformSettings = localPlatformSettings;
        _logger = logger;
    }

    private static readonly List<Regex> _noProxies =
        new()
        {
            new Regex("^/$"),
            new Regex("^/Home/"),
            new Regex("^/localtestresources/"),
            new Regex("^/LocalPlatformStorage/"),
            new Regex("^/authentication/"),
            new Regex("^/authorization/"),
            new Regex("^/profile/"),
            new Regex("^/events/"),
            new Regex("^/register/"),
            new Regex("^/storage/"),
        };

    static readonly HttpClient _client = new HttpClient(
        new HttpClientHandler { UseCookies = false, AllowAutoRedirect = false }
    );

    public async Task Invoke(HttpContext context)
    {
        var path = context.Request.Path.Value;
        if (path == null)
        {
            await _nextMiddleware(context);
            return;
        }
        foreach (var noProxy in _noProxies)
        {
            if (noProxy.IsMatch(path))
            {
                await _nextMiddleware(context);
                return;
            }
        }
        await ProxyRequest(context, localPlatformSettings.Value.LocalAppUrl);
        return;
    }

    public async Task ProxyRequest(HttpContext context, string newHost)
    {
        var request = CreateTargetMessage(context, newHost);
        context.Response.Headers.Append(
            "X-Altinn-localtest-redirect",
            request.RequestUri?.ToString()
        );
        using var response = await _client.SendAsync(
            request,
            HttpCompletionOption.ResponseHeadersRead
        );
        context.Response.StatusCode = (int)response.StatusCode;
        CopyFromTargetResponseHeaders(context, response);
        _logger.LogInformation(
            "Proxying response status {status} from {method} {uri} ",
            response.StatusCode,
            request.Method,
            request.RequestUri
        );
        await response.Content.CopyToAsync(context.Response.Body);
    }

    private static HttpRequestMessage CreateTargetMessage(HttpContext context, string newHost)
    {
        HttpRequestMessage requestMessage =
            new()
            {
                RequestUri = new($"{newHost}{context.Request.Path}{context.Request.QueryString}"),
                Method = new HttpMethod(context.Request.Method),
            };

        if (context.Request.ContentLength > 0)
        {
            requestMessage.Content = new StreamContent(context.Request.Body);
        }

        foreach (var header in context.Request.Headers)
        {
            if (
                requestMessage.Content is not null
                && (header.Key == "Content-Type" || header.Key == "Content-Disposition")
            )
            {
                requestMessage.Content.Headers.TryAddWithoutValidation(
                    header.Key,
                    header.Value.ToArray()
                );
            }
            else
            {
                requestMessage.Headers.TryAddWithoutValidation(header.Key, header.Value.ToArray());
            }
        }
        requestMessage.Headers.Host = new Uri(newHost).Host;
        return requestMessage;
    }

    private static bool RequestMethodUsesBody(string method) =>
        !(
            HttpMethods.IsGet(method)
            || HttpMethods.IsHead(method)
            || HttpMethods.IsDelete(method)
            || HttpMethods.IsTrace(method)
        );

    private static void CopyFromTargetResponseHeaders(
        HttpContext context,
        HttpResponseMessage responseMessage
    )
    {
        foreach (var header in responseMessage.Headers)
        {
            context.Response.Headers[header.Key] = header.Value.ToArray();
        }

        foreach (var header in responseMessage.Content.Headers)
        {
            context.Response.Headers[header.Key] = header.Value.ToArray();
        }
        context.Response.Headers.Remove("transfer-encoding");
    }
}
