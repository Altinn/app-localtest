using System.Text.Json.Serialization;
using LocalTest.Notifications.Core.Models.Enums;

namespace LocalTest.Notifications.Core.Models.Address;

/// <summary>
/// Interface describing an address point
/// </summary>
[JsonDerivedType(typeof(EmailAddressPoint), "email")]
[JsonPolymorphic(TypeDiscriminatorPropertyName = "$")]
public interface IAddressPoint
{
    /// <summary>
    /// Gets or sets the address type for the address point
    /// </summary>
    public AddressType AddressType { get; }
}
