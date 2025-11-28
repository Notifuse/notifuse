import awsS3Logo from '../../assets/s3-providers/aws-s3.png'
import cloudflareLogo from '../../assets/s3-providers/cloudflare.png'
import digitaloceanLogo from '../../assets/s3-providers/digitalocean.png'
import backblazeLogo from '../../assets/s3-providers/backblaze.png'
import wasabiLogo from '../../assets/s3-providers/wasabi.png'
import minioLogo from '../../assets/s3-providers/minio.png'
import scalewayLogo from '../../assets/s3-providers/scaleway.png'
import linodeLogo from '../../assets/s3-providers/linode.png'
import googleCloudLogo from '../../assets/s3-providers/google-cloud.png'
import hetznerLogo from '../../assets/s3-providers/hetzner.png'
import otherLogo from '../../assets/s3-providers/other.svg'

export interface S3Provider {
  id: string
  name: string
  logo: string
  endpointTemplate: string
  endpointPlaceholder: string
  endpointHelp?: string
  regionRequired: boolean
  regionOptions?: string[]
  regionPlaceholder?: string
  defaultRegion?: string
  forcePathStyle: boolean
  showForcePathStyle: boolean
}

export const S3_PROVIDERS: S3Provider[] = [
  {
    id: 'aws',
    name: 'Amazon S3',
    logo: awsS3Logo,
    endpointTemplate: 'https://s3.{region}.amazonaws.com',
    endpointPlaceholder: 'https://s3.us-east-1.amazonaws.com',
    regionRequired: true,
    regionPlaceholder: 'us-east-1',
    regionOptions: [
      'us-east-1',
      'us-east-2',
      'us-west-1',
      'us-west-2',
      'eu-west-1',
      'eu-west-2',
      'eu-west-3',
      'eu-central-1',
      'eu-north-1',
      'ap-northeast-1',
      'ap-northeast-2',
      'ap-northeast-3',
      'ap-southeast-1',
      'ap-southeast-2',
      'ap-south-1',
      'sa-east-1',
      'ca-central-1'
    ],
    forcePathStyle: false,
    showForcePathStyle: false
  },
  {
    id: 'google-cloud',
    name: 'Google Cloud Storage',
    logo: googleCloudLogo,
    endpointTemplate: 'https://storage.googleapis.com',
    endpointPlaceholder: 'https://storage.googleapis.com',
    endpointHelp: 'Uses S3-compatible interoperability mode',
    regionRequired: false,
    forcePathStyle: false,
    showForcePathStyle: false
  },
  {
    id: 'digitalocean-spaces',
    name: 'DigitalOcean Spaces',
    logo: digitaloceanLogo,
    endpointTemplate: 'https://{region}.digitaloceanspaces.com',
    endpointPlaceholder: 'https://nyc3.digitaloceanspaces.com',
    regionRequired: true,
    regionPlaceholder: 'nyc3',
    regionOptions: ['nyc3', 'ams3', 'sgp1', 'fra1', 'sfo2', 'sfo3', 'syd1'],
    forcePathStyle: false,
    showForcePathStyle: false
  },
  {
    id: 'cloudflare-r2',
    name: 'Cloudflare R2',
    logo: cloudflareLogo,
    endpointTemplate: 'https://{accountId}.r2.cloudflarestorage.com',
    endpointPlaceholder: 'https://abc123def456.r2.cloudflarestorage.com',
    endpointHelp: 'Replace with your Cloudflare Account ID',
    regionRequired: false,
    defaultRegion: 'auto',
    forcePathStyle: true,
    showForcePathStyle: false
  },
  {
    id: 'hetzner',
    name: 'Hetzner',
    logo: hetznerLogo,
    endpointTemplate: 'https://{region}.your-objectstorage.com',
    endpointPlaceholder: 'https://fsn1.your-objectstorage.com',
    regionRequired: true,
    regionPlaceholder: 'fsn1',
    regionOptions: ['fsn1', 'nbg1', 'hel1'],
    forcePathStyle: false,
    showForcePathStyle: false
  },
  {
    id: 'minio',
    name: 'MinIO',
    logo: minioLogo,
    endpointTemplate: '',
    endpointPlaceholder: 'https://minio.yourdomain.com',
    endpointHelp: 'Your self-hosted MinIO endpoint',
    regionRequired: false,
    defaultRegion: 'us-east-1',
    forcePathStyle: true,
    showForcePathStyle: true
  },
  {
    id: 'scaleway',
    name: 'Scaleway',
    logo: scalewayLogo,
    endpointTemplate: 'https://s3.{region}.scw.cloud',
    endpointPlaceholder: 'https://s3.fr-par.scw.cloud',
    regionRequired: true,
    regionPlaceholder: 'fr-par',
    regionOptions: ['fr-par', 'nl-ams', 'pl-waw'],
    forcePathStyle: false,
    showForcePathStyle: false
  },
  {
    id: 'wasabi',
    name: 'Wasabi',
    logo: wasabiLogo,
    endpointTemplate: 'https://s3.{region}.wasabisys.com',
    endpointPlaceholder: 'https://s3.us-east-1.wasabisys.com',
    regionRequired: true,
    regionPlaceholder: 'us-east-1',
    regionOptions: [
      'us-east-1',
      'us-east-2',
      'us-central-1',
      'us-west-1',
      'eu-central-1',
      'eu-central-2',
      'eu-west-1',
      'eu-west-2',
      'ap-northeast-1',
      'ap-northeast-2',
      'ap-southeast-1',
      'ap-southeast-2'
    ],
    forcePathStyle: true,
    showForcePathStyle: false
  },
  {
    id: 'backblaze-b2',
    name: 'Backblaze B2',
    logo: backblazeLogo,
    endpointTemplate: 'https://s3.{region}.backblazeb2.com',
    endpointPlaceholder: 'https://s3.us-west-004.backblazeb2.com',
    endpointHelp: 'Find your region in the B2 bucket details',
    regionRequired: true,
    regionPlaceholder: 'us-west-004',
    forcePathStyle: false,
    showForcePathStyle: false
  },
  {
    id: 'linode',
    name: 'Linode',
    logo: linodeLogo,
    endpointTemplate: 'https://{region}.linodeobjects.com',
    endpointPlaceholder: 'https://us-east-1.linodeobjects.com',
    regionRequired: true,
    regionPlaceholder: 'us-east-1',
    regionOptions: [
      'us-east-1',
      'eu-central-1',
      'ap-south-1',
      'us-southeast-1',
      'us-ord-1',
      'fr-par-1',
      'us-sea-1',
      'br-gru-1',
      'us-mia-1',
      'id-cgk-1',
      'in-maa-1',
      'jp-osa-1',
      'it-mil-1',
      'se-sto-1',
      'au-mel-1'
    ],
    forcePathStyle: false,
    showForcePathStyle: false
  },
  {
    id: 'other',
    name: 'Other S3-Compatible',
    logo: otherLogo,
    endpointTemplate: '',
    endpointPlaceholder: 'https://your-s3-endpoint.com',
    endpointHelp: 'Any S3-compatible storage service',
    regionRequired: false,
    regionPlaceholder: 'us-east-1',
    forcePathStyle: true,
    showForcePathStyle: true
  }
]

export function getProviderById(id: string): S3Provider | undefined {
  return S3_PROVIDERS.find((p) => p.id === id)
}

export function generateEndpoint(provider: S3Provider, region?: string): string {
  if (!provider.endpointTemplate) {
    return ''
  }

  let endpoint = provider.endpointTemplate

  if (region && endpoint.includes('{region}')) {
    endpoint = endpoint.replace('{region}', region)
  }

  return endpoint
}
