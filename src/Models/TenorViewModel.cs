#nullable enable
namespace LocalTest.Models;

using LocalTest.Services.Tenor.Models;
using LocalTest.Services.TestData;

public class TenorViewModel
{
    public List<TenorFileItem> FileItems { get; set; } = default!;
    public AppTestDataModel? AppUsers { get; set; }
}

public class TenorFileItem
{
    public string FileName { get; set; } = default!;
    public string RawFileContent { get; set; } = default!;
    public Freg? Freg { get; set; }
    public BrregErFr? Brreg { get; set; }
}