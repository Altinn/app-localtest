## Local testing of apps

These are some of the required steps, tips, and tricks when it comes to running an app on a local machine. The primary goal is to be able to iterate over changes and verifying them without needing to deploy the app to the test environment.

- [Prerequisites](#prerequisites)
- [Setup](#setup)
  - [Clone the repository](#clone-the-repository)
  - [Option A: Start the containers using podman](#option-a-start-the-containers-using-podman)
  - [Option B: Start the containers using Docker](#option-b-start-the-containers-using-docker)
  - [Option C (preview): Automatic detection](#option-c-preview-automatic-detection)
  - [Start your app](#start-your-app)
- [Changing configuration](#changing-configuration)
- [Multiple apps at the same time (running LocalTest locally)](#multiple-apps-at-the-same-time-running-localtest-locally)
- [Changing test data](#changing-test-data)
  - [Add a missing role for a test user](#add-a-missing-role-for-a-test-user)
- [Known issues](#known-issues)
  - [Localtest reports that the app is not running even though it is](#localtest-reports-that-the-app-is-not-running-even-though-it-is)

### Prerequisites

1. .NET SDK matching your service (the latest App Template uses [.NET 8 SDK](https://dotnet.microsoft.com/download/dotnet/8.0), older versions may also require [.NET 6 SDK](https://dotnet.microsoft.com/download/dotnet/6.0))
2. Newest [Git](https://git-scm.com/downloads)
3. A code editor - we like [Visual Studio Code](https://code.visualstudio.com/Download)
    - Also
      install [recommended extensions](https://code.visualstudio.com/docs/editor/extension-gallery#_workspace-recommended-extensions) (
      f.ex. [C#](https://marketplace.visualstudio.com/items?itemName=ms-vscode.csharp))
4. Tooling to run containers:  
    - [Podman](https://podman.io)* (optionally with [Podman Desktop](https://podman-desktop.io))
    - [Docker Desktop](https://www.docker.com/products/docker-desktop) (Windows and Mac) This might require you to purchase a license. 
    - Linux/WSL users can also use native Docker.
  
 > [!NOTE] 
 > On Mac with Apple silicone (M-series CPU), [vfkit](https://github.com/crc-org/vfkit?tab=readme-ov-file#installation) might be needed - consult the install guide/requirements for your container toolkit


> [!WARNING]
> *Podman on Windows/WSL2 can be tricky. If faced with the same issue as described in https://github.com/Altinn/app-localtest/issues/84, please apply the following (tested on Podman 4.9.2 and 5.1.1):
> * Install the Podman Machine with "user-mode networking" enabled (the setting for root/rootless seems not to have an impact)
> * Apply local updates to `podman-compose.yml` and `src\appsettings.Podman.json` replacing `host.docker.internal` with `<your-hostname>.local` (obtained by running `hostname` in the windows host commandline).  
>   e.g. in `src\appsettings.Podman.json` the config `"LocalAppUrl": "http://host.docker.internal:5005",` becomes `"LocalAppUrl": "http://AAD-123456789.local:5005",` if your hostname is `AAD-123456789`
> 
> If you have special networking needs (VPN), additional settings might be needed in WSL2/Podman. Please consult their respective doc/forums. https://learn.microsoft.com/en-us/windows/wsl/wsl-config#wslconfig and https://github.com/containers/podman/blob/main/docs/tutorials/basic_networking.md are good starting-ponts.

### Setup

#### Clone the repository

```shell
git clone https://github.com/Altinn/app-localtest
cd app-localtest
 ```

#### Option A: Start the containers using podman

This mode supports running one app at a time. If you need to run multiple apps at once, stop the localtest container with `podman stop localtest` and follow the instructions below to run LocalTest locally outside Docker/Podman.

> [!IMPORTANT]
> If you are using an mac with either a M1, M2 or M3 chip you may need to use `applehv` instead of `qemu` as the podman machine driver. 
> This can be done by setting the environment variable `CONTAINERS_MACHINE_PROVIDER` to `applehv` before running the command below.
> To add this to your zsh profile run the following command: `echo "export CONTAINERS_MACHINE_PROVIDER=applehv" >> ~/.zprofile`
> If you are using Podman Desktop you also need to add these lines in `~/.config/containers/containers.conf` (check if the `[machine]` section already exists):
>  
> ```
> [machine]
>   provider = "applehv"
> ```

Start the containers with the following command:

```shell
podman compose --file podman-compose.yml up -d --build
```

Optionally, if you want access to Grafana and a local monitoring setup based on OpenTelemetry:

```shell
podman compose --file podman-compose.yml --profile "monitoring" up -d --build
# Grafana should be available at http://local.altinn.cloud:8000/grafana
# Remember to enable the 'UseOpenTelemetry' configuration flag in the appsettings.json of the app
```

> [!NOTE]
> If you are using linux or mac you can use the Makefile to build and run the containers.
> 
> ```shell
> make podman-start-localtest
> ```

> [!IMPORTANT]
> Are you running podman version < 4.7.0 you need to use the following command instead:
> 
> ```shell
> podman-compose --file podman-compose.yml up -d --build
> ```
> 
> or the make command:
> 
> ```shell
> make podman-compose-start-localtest
> ```

Localtest should now be runningn on port 8000 and can be accessed on <http://local.altinn.cloud:8000>.

#### Option B: Start the containers using Docker

This mode supports running one app at a time. If you need to run multiple apps at once, stop the localtest container with `docker stop localtest` and follow the instructions below to run LocalTest locally outside Docker.

```shell
docker compose up -d --build
```

Optionally, if you want access to Grafana and a local monitoring setup based on OpenTelemetry:

```shell
docker compose --profile "monitoring" up -d --build
# Grafana should be available at http://local.altinn.cloud/grafana
# Remember to enable the 'UseOpenTelemetry' configuration flag in the appsettings.json of the app
```
   
> [!NOTE]
> If you are using linux or mac you can use the Makefile to build and run the containers.
> 
> ```shell
> make docker-start-localtest
> ```

Localtest should now be runningn on port 80 and can be accessed on <http://local.altinn.cloud:80>.

#### Option C (preview): Automatic detection

There is a preview helper script that will execute the correct commands in a cross-platform way.
Either docker or podman must be installed.

```shell
./run.cmd
```

Optionally, if you want access to Grafana and a local monitoring setup based on OpenTelemetry:

```shell
./run.cmd -m
```

If the localtest setup is already running, it will restart.

To stop localtest
```shell
./run.cmd stop
```

#### Start your app

_This step requires that you have already [set up an app](https://docs.altinn.studio/nb/altinn-studio/guides/development/basic-form/) and [cloned the app](https://docs.altinn.studio/nb/altinn-studio/guides/development/local-dev/) to your local environment._

Move into the `App` folder of your application.

Example: If your application is named `my-awesome-app` and is located in the folder `C:\my_applications`, run the following command:

```shell
cd C:\my_applications\my-awasome-app\App
```

Run the application:

 ```shell
 dotnet run
```

The app and local platform services are now running locally. The app can be accessed on <http://local.altinn.cloud>.

Log in with a test user, using your app name and org name. This will redirect you to the app.

### Changing configuration

The Docker Compose config can be changed with local environment variables. There is a
template file for the `.env` file [here](./.env.template), rename it to `.env` and
uncomment some variables that you want different values for.

Sometimes the local environment have another service running on port 80, so you might need to change this.

```dotenv
ALTINN3LOCAL_PORT=80
```

If you want to see the storage files on disk (instead of reading them through the browser), change this to a local
path on your computer (ensure that it exists)

```dotenv
ALTINN3LOCALSTORAGE_PATH=C:/AltinnPlatformLocal/
```

If you want to use another domain than `local.altinn.cloud` for local testing you could do that this way:

```dotenv
TEST_DOMAIN=local.altinn.cloud
```

### Multiple apps at the same time (running LocalTest locally)

The setup described above (LocalTest running in Docker) currently only supports one app at a time. If you find
yourself needing to run multiple apps at the same time, or if you need to debug or develop LocalTest, a local setup is
preferred.

:information_source: If you're already running LocalTest in Docker, be sure to stop the container with `docker stop localtest`

**Configuration of LocalTest**
The LocalTest application acts as an emulator of the Altinn 3 platform services. It provides things like authentication,
authorization and storage. Everything your apps will need to run locally.

Settings (under `LocalPlatformSettings`):

- `LocalAppMode` - (default `file`) If set to `http`, LocalTest will find the active app configuration and policy.xml
  using apis exposed on `LocalAppUrl`. (note that this is a new setting needs to be added manually under
  `LocalPlatformSettings`, it might also require updates to altinn dependencies for your apps in order to support
  this functionality)
- `LocalAppUrl` - If `LocalAppMode` == `"http"`, this URL will be used instead of `AppRepositoryBasePath` to find apps
  and their files. Typically the value will be `"http://localhost:5005"`
- `LocalTestingStorageBasePath` - The folder to which LocalTest will store instances and data being created
  during testing.
- `AppRepositoryBasePath` - The folder where LocalTest will look for apps and their files
  if `LocalAppMode` == `"file"`. This is typically the parent directory where you checkout all your apps.
- `LocalTestingStaticTestDataPath` - Test user data like profile, register and
  roles. (`<path to altinn-studio repo>/testdata/`)

The recommended way of changing settings for LocalTest is through
[user-secrets](https://docs.microsoft.com/en-us/aspnet/core/security/app-secrets?view=aspnetcore-6.0&tabs=windows#set-a-secret).
User secrets is a set of developer specific settings that will overwrite values from the `appsettings.json` file when
the application is started in developer "mode". The alternative is to edit the `appsettings.json` file directly. Just be
careful not to commit developer specific changes back to the repository.

- Define a user secret with the following command:  (make sure you are in the LocalTest folder)
   ```bash
   dotnet user-secrets set "LocalPlatformSettings:AppRepositoryBasePath" "C:\Repos"
   ```
  Run the command for each setting you want to change.
- Alternatively edit the appsettings.json file directly:
    - Open `appsettings.json` in the `LocalTest` folder in an editor, for example in Visual Studio Code
    - Change the setting `"AppRepsitoryBasePath"` to the full path to your app on the disk.
    - Change other settings as needed.
    - Save the file.

Finally, start the local platform services (make sure you are in the `/src` folder)

```bash
cd /src
dotnet run
```

### Changing test data

In some cases your application might differ from the default setup and require custom changes to the test data
available.
This section contains the most common changes.

#### Add a missing role for a test user

This would be required if your app requires a role which none of the test users have.

1. Identify the role list you need to modify by noting the userId of the user representing an entity, and the partyId of
   the entity you want to represent
2. Find the correct `roles.json` file in `testdata/authorization/roles` by navigating
   to `User_{userID}\party_{partyId}\roles.json`
3. Add a new entry in the list for the role you require

  ```
  {
    "Type": "altinn",
    "value": "[Insert role code here]"
  }
  ```

4. Save and close the file
5. Restart LocalTest

### k6 testing

In the k6 folder there is a sample loadtest that can be adapted to run automated tests against a local app
It was created to simulate workloads and test monitoring and instrumentation.

```shell
cp k6/loadtest.sample.js k6/loadtest.js
# Now make edits to k6/loadtest.js to fit your application

# To run, either
./run.cmd k6
# or 
docker run --rm -i --net=host grafana/k6:master-with-browser run - <k6/loadtest.js
```

For a decent editing experience, run `npm install` and use a editor with JS support.

### Known issues

#### Localtest reports that the app is not running even though it is

If localtest and you app is running, but localtest reports that the app is not running, it might be that the port is not open in the firewall.

You can verify if the app is running by opening `http://localhost:5005/<app-org-name>/<app-name>/swagger/index.html` (remember to replace `<app-org-name>` and `<app-name>` with the correct values).

If this is the case you can open a Windows Powershell as administrator and run the script `OpenAppPortInHyperVFirewall.ps1` located in the `scripts` folder.
