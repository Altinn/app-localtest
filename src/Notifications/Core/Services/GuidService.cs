using System.Diagnostics.CodeAnalysis;

namespace LocalTest.Notifications.Core.Services;

/// <summary>
/// Implementation of the GuidServiceS
/// </summary>
[ExcludeFromCodeCoverage]
public class GuidService : IGuidService
{
    /// <inheritdoc/>
    public Guid NewGuid()
    {
        return Guid.NewGuid();
    }
}
