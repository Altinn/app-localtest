#nullable enable
using Microsoft.AspNetCore.Mvc;

namespace LocalTest.Controllers;

/// <summary>
/// Simple controller to return the API version,
/// so that app-lib can require a specific version.
/// </summary>
[ApiController]
public class ApiVersionController : Controller
{
    [HttpGet("storage/api/v1/api-version")]
    public string Index()
    {
        return "1";
    }
}
