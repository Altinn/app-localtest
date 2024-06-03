#nullable enable
using LocalTest.Configuration;
using LocalTest.Models;
using LocalTest.Services.LocalFrontend.Interface;
using Microsoft.Extensions.Options;
using static System.Linq.Enumerable;

namespace LocalTest.Services.LocalFrontend;

public class LocalFrontendService : ILocalFrontendService
{
    private readonly HttpClient _httpClient;
    private static readonly Range PortRange = 8080..8090;
    private readonly string _localFrontedBaseUrl;

    public LocalFrontendService(
        IHttpClientFactory httpClientFactory,
        IOptions<LocalPlatformSettings> localPlatformSettings
    )
    {
        _httpClient = httpClientFactory.CreateClient();
        _httpClient.Timeout = TimeSpan.FromMilliseconds(500);
        _localFrontedBaseUrl =
            $"{localPlatformSettings.Value.LocalFrontendProtocol}://{localPlatformSettings.Value.LocalFrontendHostname}";
    }

    public async Task<List<LocalFrontendInfo>> GetLocalFrontendDevPorts()
    {
        var tasks = Range(PortRange.Start.Value, PortRange.End.Value).Select(port => TestFrontendDevPort(port));
        var result = await Task.WhenAll(tasks);
        return result.Where(x => x != null).Select(x => x!.Value).ToList();
    }

    private async Task<LocalFrontendInfo?> TestFrontendDevPort(int port)
    {
        try
        {
            var response = await _httpClient.GetAsync($"{_localFrontedBaseUrl}:{port.ToString()}/");
            if (response.Headers.TryGetValues("X-Altinn-Frontend-Branch", out var values))
            {
                return new LocalFrontendInfo()
                {
                    Port = port.ToString(),
                    Branch = values?.First() ?? "Unknown"
                };
            }
        }
        catch (TaskCanceledException) { }
        catch (HttpRequestException) { }

        return null;
    }
}

