import { initialState } from '../reducer'
import { sePackageTag, DEBUG_API_SERVER } from 'components/DbLabInstanceForm/utils'

const API_SERVER = process.env.REACT_APP_API_SERVER

export const getPlaybookCommand = (
  state: typeof initialState,
  orgKey: string,
) =>
  `docker run --rm -it \\\r
    -v $HOME/.ssh:/root/.ssh:ro -e ANSIBLE_SSH_ARGS="-F none" \\\r
    postgresai/dle-se-ansible:${sePackageTag} \\\r
    ansible-playbook deploy_dle.yml --extra-vars \\\r
      "dle_host='user@server-ip-address' \\\r
      dle_platform_project_name='${state.name}' \\\r
      dle_version='${state.tag}' \\\r
      ${orgKey ? `dle_platform_org_key='${orgKey}' \\\r` : ``}
      ${
      API_SERVER === DEBUG_API_SERVER
        ? `dle_platform_url='${DEBUG_API_SERVER}' \\\r`
        : ``
    }
      ${state.publicKeys ? `ssh_public_keys='${state.publicKeys}' \\\r` : ``}
      dle_verification_token='${state.verificationToken}'"
`

export const getAnsiblePlaybookCommand = (
  state: typeof initialState,
  orgKey: string,
) =>
  `ansible-playbook deploy_dle.yml --extra-vars \\\r
  "dle_host='user@server-ip-address' \\\r
  dle_platform_project_name='${state.name}' \\\r
  dle_version='${state.tag}' \\\r
  ${orgKey ? `dle_platform_org_key='${orgKey}' \\\r` : ``}
  ${
    API_SERVER === DEBUG_API_SERVER
      ? `dle_platform_url='${DEBUG_API_SERVER}' \\\r`
      : ``
  }
  ${state.publicKeys ? `ssh_public_keys='${state.publicKeys}' \\\r` : ``}
  dle_verification_token='${state.verificationToken}'"
`

export const getAnsibleInstallationCommand = () =>
  `sudo apt update
sudo apt install -y python3-pip
pip3 install ansible`

export const cloneRepositoryCommand = () =>
  `git clone --depth 1 --branch ${sePackageTag} \\
    https://gitlab.com/postgres-ai/dle-se-ansible.git \\
  && cd dle-se-ansible/`
