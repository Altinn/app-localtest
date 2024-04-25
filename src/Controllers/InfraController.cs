using Microsoft.AspNetCore.Mvc;

namespace LocalTest.Controllers;

[ApiController]
[Route("Home/[controller]/[action]")]
public class InfraController : ControllerBase
{
    private readonly ILogger<InfraController> _logger;
    private readonly IHttpClientFactory _httpClientFactory;

    public InfraController(ILogger<InfraController> logger, IHttpClientFactory httpClientFactory)
    {
        _logger = logger;
        _httpClientFactory = httpClientFactory;
    }

    [HttpGet]
    public async Task<IActionResult> Grafana(CancellationToken cancellationToken)
    {
        using var client = _httpClientFactory.CreateClient();
        client.Timeout = TimeSpan.FromSeconds(2);

        try
        {
            // TODO: how to run locally/on host? Different domain then (localhost)?
            var response = await client.GetAsync("http://monitoring_grafana:3000/api/health", cancellationToken);
            if (response.IsSuccessStatusCode)
            {
                return Ok();
            }
            else
            {
                throw new Exception("Unexpected status code: " + response.StatusCode);
            }
        }
        catch (Exception ex)
        {
            _logger.LogWarning(ex, "Error while checking health of {container}", "grafana");
            return StatusCode(StatusCodes.Status503ServiceUnavailable);
        }
    }
}
