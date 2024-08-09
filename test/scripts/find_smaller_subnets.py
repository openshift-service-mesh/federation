import ipaddress
import argparse
import sys

def find_subnet_with_required_mask(network):
    try:
        # Parse the network address
        net = ipaddress.ip_network(network, strict=False)

        # Determine the required new mask
        current_mask = net.prefixlen
        # We need 2 subnets, and we leave the first one to avoid potential overlapping
        required_subnets = 4
        new_mask = current_mask

        # Calculate the new mask to provide at least 4 subnets
        while (2 ** (new_mask - current_mask)) < required_subnets:
            new_mask += 1

        # Generate subnets with the new mask
        subnets = list(net.subnets(new_prefix=new_mask))

        # Ensure at least 4 subnets are available
        if len(subnets) < required_subnets:
            raise ValueError(f"Unable to create {required_subnets} subnets with mask {new_mask}.")

        return subnets[1], subnets[2]
    except ValueError as e:
        print(f"Error: {e}")
        return None

def main():
    # Set up argument parsing
    parser = argparse.ArgumentParser(description='Find a CIDR range for a given network address and mask.')
    parser.add_argument('--network', type=str, help='The network address in CIDR notation (e.g., 172.18.0.0/16)')
    parser.add_argument('--region', type=str, help='The k8s cluster region')

    # Parse the arguments
    args = parser.parse_args()

    if args.region != "west" and args.region != "east":
        print(f"unknown region: {args.region}")
        sys.exit(1)

    # Find the required subnet
    west_subnet, east_subnet = find_subnet_with_required_mask(args.network)

    if not west_subnet or not east_subnet:
        print(f"failed to generate subnets: [west-subnet={west_subnet},east-subnet={east_subnet}]")
        sys.exit(1)

    if args.region == "west":
        print(west_subnet)
    else:
        print(east_subnet)

if __name__ == "__main__":
    main()