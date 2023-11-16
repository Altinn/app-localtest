namespace LocalTest.Notifications.Core.Services;

/// <summary>
/// Interface describing a guid service
/// </summary>
public interface IGuidService
{
    /// <summary>
    /// Generates a new Guid
    /// </summary>
    public Guid NewGuid();
}
