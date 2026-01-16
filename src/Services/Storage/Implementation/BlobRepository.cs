using Altinn.Platform.Storage.Interface.Models;
using LocalTest.Services.Storage.Implementation;

namespace Altinn.Platform.Storage.Repository;

public class BlobRepository : IBlobRepository
{
    public async Task<bool> DeleteBlob(
        string org,
        string blobStoragePath,
        int? storageAccountNumber
    )
    {
        return await Task.FromResult(true);
    }

    public async Task<Stream> ReadBlob(
        string org,
        string blobStoragePath,
        int? storageAccountNumber,
        CancellationToken cancellationToken = default
    )
    {
        string dataPath = Path.Combine(GetDataBlobPath(), blobStoragePath);
        Stream fs = File.OpenRead(dataPath);

        return await Task.FromResult(fs);
    }

    public async Task<(long ContentLength, DateTimeOffset LastModified)> WriteBlob(
        string org,
        Stream stream,
        string blobStoragePath,
        int? storageAccountNumber
    )
    {
        MemoryStream memoryStream = new MemoryStream();
        await stream.CopyToAsync(memoryStream);
        return (memoryStream.Length, DateTimeOffset.UtcNow);
    }

    private static string GetDataBlobPath()
    {
        string unitTestFolder = Path.GetDirectoryName(
            new Uri(typeof(DataRepository).Assembly.Location).LocalPath
        );
        return Path.Combine(unitTestFolder, "..", "..", "..", "data", "blob");
    }

    public Task<bool> DeleteDataBlobs(Instance instance, int? storageAccountNumber)
    {
        throw new NotImplementedException();
    }
}
