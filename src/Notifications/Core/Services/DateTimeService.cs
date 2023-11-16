using System.Diagnostics.CodeAnalysis;

namespace LocalTest.Notifications.Core.Services;

/// <summary>
/// Implemntation of a dateTime service
/// </summary>
[ExcludeFromCodeCoverage]
public class DateTimeService : IDateTimeService
{
    /// <inheritdoc/>
    public DateTime UtcNow()
    {
        return DateTime.UtcNow;
    }
}
