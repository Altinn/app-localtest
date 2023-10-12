#nullable enable
using System.Text.Json;

using Microsoft.AspNetCore.Mvc;
using Microsoft.Extensions.Options;

using Altinn.Platform.Storage.Repository;

using LocalTest.Configuration;
using LocalTest.Models;
using LocalTest.Services.LocalApp.Interface;

using LocalTest.Services.TestData;
using Microsoft.AspNetCore.Authorization;

namespace LocalTest.Controllers;

[Route("Home/[controller]/[action]")]
public class StorageExplorerController : Controller
{
    private readonly TenorDataRepository _tenorDataRepository;
    private readonly LocalPlatformSettings _localPlatformSettings;
    private readonly ILocalApp _localApp;

    public StorageExplorerController(
        TenorDataRepository tenorDataRepository,
        IOptions<LocalPlatformSettings> localPlatformSettings,
        ILocalApp localApp)
    {
        _tenorDataRepository = tenorDataRepository;
        _localPlatformSettings = localPlatformSettings.Value;
        _localApp = localApp;
    }

    public IActionResult Index()
    {
        return View();
    }
}