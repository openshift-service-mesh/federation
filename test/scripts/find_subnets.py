import ipaddress
import argparse
import math
import sys

def find_subnets(network, required_subnets):
    try:
        # Create additional subnets to avoid overlapping
        required_subnets += 2
        bits_needed = math.ceil(math.log2(required_subnets))
        
        network = ipaddress.ip_network(network, strict=False)

        new_mask = network.prefixlen + bits_needed
        if new_mask > network.max_prefixlen:
            raise ValueError("Cannot create that many subnets with the given network.")

        return list(network.subnets(new_prefix=new_mask))[1:required_subnets-1]

    except ValueError as e:
        print(f"Error: {e}")
        sys.exit(1)

def main():
    parser = argparse.ArgumentParser(description='Find CIDR subnets for a given network address.')
    parser.add_argument('--network', type=str, required=True, help='The network address in CIDR notation (e.g., 172.18.0.0/16)')
    parser.add_argument('--region', type=str, required=True, help='The k8s cluster region (e.g., west, east, etc.)')
    parser.add_argument('--regions', nargs='+', required=True, help='Comma-separated list of regions (e.g., west,east,central)')

    args = parser.parse_args()

    if args.region not in args.regions:
        print(f"Unknown region: {args.region}")
        sys.exit(1)

    required_subnets = len(args.regions)
    subnets = find_subnets(args.network, required_subnets)

    if not subnets or len(subnets) < required_subnets:
        print(f"Failed to generate subnets: {subnets}")
        sys.exit(1)

    region_index = args.regions.index(args.region)
    print(subnets[region_index])

if __name__ == "__main__":
    main()
