#nullable enable
using System.Text.Json;

using Microsoft.AspNetCore.Mvc;
using Microsoft.Extensions.Options;

using Altinn.Platform.Storage.Repository;

using LocalTest.Configuration;
using LocalTest.Models;
using LocalTest.Services.LocalApp.Interface;

using LocalTest.Services.TestData;

namespace LocalTest.Controllers;

[Route("Home/[controller]/[action]")]
public class TenorUsersController : Controller
{
    private readonly TenorDataRepository _tenorDataRepository;
    private readonly ILocalApp _localApp;

    public TenorUsersController(
        TenorDataRepository tenorDataRepository,
        ILocalApp localApp)
    {
        _tenorDataRepository = tenorDataRepository;
        _localApp = localApp;
    }



    public async Task<IActionResult> Index()
    {
        var appUsers = await _localApp.GetTestData();

        return View(new TenorViewModel()
        {
            AppUsers = appUsers,
            FileItems = await _tenorDataRepository.GetFileItems(),

        });
    }



    [HttpPost]
    [ValidateAntiForgeryToken]
    public async Task<IActionResult> Upload()
    {
        //TODO: validate uploaded files
        foreach (var file in Request.Form.Files)
        {
            await _tenorDataRepository.StoreUploadedFile(file);
        }
        return RedirectToAction("Index");
    }

    [HttpPost]
    [ValidateAntiForgeryToken]
    public async Task<IActionResult> Update()
    {
        var files = Request.Form.Keys.Where(k => k.EndsWith(".json")).ToList();
        if (Request.Form.ContainsKey("Download"))
        {
            return Json(await _tenorDataRepository.GetAppTestDataModel(files));
        }
        else if (Request.Form.ContainsKey("DownloadFile"))
        {
            return File(JsonSerializer.SerializeToUtf8Bytes(await _tenorDataRepository.GetAppTestDataModel(files)), "application/json", "testData.json");
        }
        else if (Request.Form.ContainsKey("Delete"))
        {
            foreach (var file in files)
            {
                _tenorDataRepository.DeleteFile(file);
            }

            return RedirectToAction("Index");
        }
        else
        {
            throw new Exception("Unknown action");
        }
    }
}