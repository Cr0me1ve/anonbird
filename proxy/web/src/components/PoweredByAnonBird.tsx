import { AnonBirdLogo } from "./AnonBirdLogo";

export function PoweredByAnonBird() {
  return (
    <a
      href="https://github.com/Cr0me1ve/netbird"
      target="_blank"
      rel="noopener noreferrer"
      className="flex items-center justify-center mt-8 gap-2 group cursor-pointer"
    >
      <span className="text-sm text-nb-gray-400 font-light text-center group-hover:opacity-80 transition-all">
        Powered by
      </span>
      <AnonBirdLogo size="small" mobile={false} />
    </a>
  );
}
