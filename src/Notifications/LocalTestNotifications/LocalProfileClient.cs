using Altinn.Notifications.Core.Integrations;
using Altinn.Notifications.Core.Models.ContactPoints;

namespace LocalTest.Notifications.LocalTestNotifications
{
    public class LocalProfileClient : IProfileClient
    {
        public Task<List<UserContactPoints>> GetUserContactPoints(List<string> nationalIdentityNumbers)
        {
            throw new NotImplementedException();
        }

        public Task<List<OrganizationContactPoints>> GetUserRegisteredOrganizationContactPoints(string resourceId, List<string> organizationNumbers)
        {
            throw new NotImplementedException();
        }
    }
}
