﻿using Altinn.Notifications.Core.Integrations;
using Altinn.Notifications.Core.Models.ContactPoints;
using Altinn.Platform.Profile.Models;

using LocalTest.Services.Register.Interface;
using LocalTest.Services.TestData;

namespace LocalTest.Notifications.LocalTestNotifications
{
    public class LocalProfileClient : IProfileClient
    {
        private readonly TestDataService _testDataService;

        public LocalProfileClient(TestDataService testDataService)
        {
            _testDataService = testDataService;
        }

        public async Task<List<UserContactPoints>> GetUserContactPoints(List<string> nationalIdentityNumbers)
        {
            List<UserContactPoints> contactPoints = new();
            var data = await _testDataService.GetTestData();


            contactPoints.AddRange(data.Profile.User
                .Where(u => nationalIdentityNumbers.Contains(u.Value.Party.SSN))
                .Select(u =>
                {
                    var user = u.Value;
                    return new UserContactPoints()
                    {
                        NationalIdentityNumber = user.Party.SSN,
                        Email = user.Email,
                        MobileNumber = user.PhoneNumber
                    };
                })
               .ToList());

            return contactPoints;

        }

        public Task<List<OrganizationContactPoints>> GetUserRegisteredOrganizationContactPoints(string resourceId, List<string> organizationNumbers)
        {
            throw new NotImplementedException();
        }
    }
}
