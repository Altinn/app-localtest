#nullable enable
using System.Text.Json;

using Microsoft.AspNetCore.Mvc;
using LocalTest.Models;
using LocalTest.Services.LocalFrontend.Interface;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Mvc.Rendering;

namespace LocalTest.Controllers;

[Route("Home/[controller]/[action]")]
public class FrontendVersionController : Controller
{
    /// <summary>
    ///  See src\development\loadbalancer\nginx.conf
    /// </summary>
    public static readonly string FRONTEND_URL_COOKIE_NAME = "frontendVersion";

    [HttpGet]
    public async Task<ActionResult> Index([FromServices] HttpClient client, [FromServices] ILocalFrontendService localFrontendService)
    {
        var versionFromCookie = HttpContext.Request.Cookies[FRONTEND_URL_COOKIE_NAME];

        var frontendVersion = new FrontendVersion()
        {
            Version = versionFromCookie,
            Versions = new List<SelectListItem>()
                {
                    new ()
                    {
                        Text = "Keep as is",
                        Value = "",
                    }
                }
        };
        var groupLocalVersions = new SelectListGroup() { Name = "Frontend served from local dev server" };
        var localFrontendPorts = await localFrontendService.GetLocalFrontendDevPorts();
        foreach (var localPort in localFrontendPorts)
        {
            frontendVersion.Versions.Add(new SelectListItem()
            {
                Text = $"Local dev-server on port {localPort.Port} ({localPort.Branch} branch)",
                Value = $"http://localhost:{localPort.Port}/",
                Group = groupLocalVersions
            });
        }
        var cdnVersionsString = await client.GetStringAsync("https://altinncdn.no/toolkits/altinn-app-frontend/index.json");
        var groupCdnVersions = new SelectListGroup() { Name = "Specific version from cdn" };
        var versions = JsonSerializer.Deserialize<List<string>>(cdnVersionsString)!;
        versions.Reverse();
        versions.ForEach(version =>
        {
            frontendVersion.Versions.Add(new()
            {
                Text = version,
                Value = $"https://altinncdn.no/toolkits/altinn-app-frontend/{version}/",
                Group = groupCdnVersions
            });
        });

        return View(frontendVersion);
    }
    public ActionResult Index(FrontendVersion frontendVersion)
    {
        var options = new CookieOptions
        {
            Expires = DateTime.MaxValue,
            HttpOnly = true,
        };
        ICookieManager cookieManager = new ChunkingCookieManager();
        if (string.IsNullOrWhiteSpace(frontendVersion.Version))
        {
            cookieManager.DeleteCookie(HttpContext, FRONTEND_URL_COOKIE_NAME, options);
        }
        else
        {
            cookieManager.AppendResponseCookie(
                HttpContext,
                FRONTEND_URL_COOKIE_NAME,
                frontendVersion.Version,
                options
                );
        }

        return RedirectToAction("Index", "Home");
    }
}