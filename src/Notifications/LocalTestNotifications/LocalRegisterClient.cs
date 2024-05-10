using Altinn.Notifications.Core.Integrations;
using Altinn.Notifications.Core.Models.ContactPoints;

namespace LocalTest.Notifications.LocalTestNotifications
{
    public class LocalRegisterClient : IRegisterClient
    {
        public Task<List<OrganizationContactPoints>> GetOrganizationContactPoints(List<string> organizationNumbers)
        {
            throw new NotImplementedException();
        }
    }
}
