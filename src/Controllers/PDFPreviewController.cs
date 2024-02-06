#nullable enable

using Microsoft.AspNetCore.Mvc;

namespace LocalTest.Controllers;

[Route("Home/[controller]/[action]")]
public class PDFPreviewController : Controller
{
    public IActionResult Index()
    {
        return View();
    }
}
